package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	contextInt "github.com/mallardduck/dirio/internal/context"
)

func TestTiming(t *testing.T) {
	t.Run("captures request start time", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		beforeRequest := time.Now()
		var capturedTime time.Time
		var found bool

		handler := Timing(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTime, found = GetRequestStartTime(r.Context())
		}))

		handler.ServeHTTP(rec, req)

		// Verify start time was captured
		if !found {
			t.Fatal("expected start time to be found in context")
		}

		// Verify timestamp is reasonable (between before request and now)
		afterRequest := time.Now()
		if capturedTime.Before(beforeRequest) || capturedTime.After(afterRequest) {
			t.Errorf("captured time %v is not between %v and %v", capturedTime, beforeRequest, afterRequest)
		}
	})

	t.Run("allows duration calculation", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		var duration time.Duration

		handler := Timing(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start, ok := GetRequestStartTime(r.Context())
			if !ok {
				t.Fatal("expected start time to be found")
			}

			// Simulate some work
			time.Sleep(10 * time.Millisecond)

			// Calculate duration
			duration = time.Since(start)
		}))

		handler.ServeHTTP(rec, req)

		// Verify duration is at least 10ms (what we slept)
		if duration < 10*time.Millisecond {
			t.Errorf("expected duration >= 10ms, got %v", duration)
		}
	})

	t.Run("works in middleware chain", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		var startTime time.Time
		var foundInMiddleware bool
		var foundInHandler bool

		// Simulate a middleware that runs after Timing
		someMiddleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				startTime, foundInMiddleware = GetRequestStartTime(r.Context())
				next.ServeHTTP(w, r)
			})
		}

		handler := Timing(someMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, foundInHandler = GetRequestStartTime(r.Context())
		})))

		handler.ServeHTTP(rec, req)

		// Verify both middleware and handler can access the start time
		if !foundInMiddleware {
			t.Error("expected start time to be found in middleware")
		}
		if !foundInHandler {
			t.Error("expected start time to be found in handler")
		}
		if startTime.IsZero() {
			t.Error("expected non-zero start time")
		}
	})
}

func TestGetRequestStartTime(t *testing.T) {
	t.Run("returns false for nil context", func(t *testing.T) {
		_, ok := GetRequestStartTime(nil)
		if ok {
			t.Error("expected ok to be false for nil context")
		}
	})

	t.Run("returns false for context without start time", func(t *testing.T) {
		ctx := context.Background()
		_, ok := GetRequestStartTime(ctx)
		if ok {
			t.Error("expected ok to be false for context without start time")
		}
	})

	t.Run("returns start time when present", func(t *testing.T) {
		expectedTime := time.Now()
		ctx := context.WithValue(context.Background(), contextInt.RequestStartTimeKey, expectedTime)

		actualTime, ok := GetRequestStartTime(ctx)
		if !ok {
			t.Fatal("expected ok to be true for context with start time")
		}
		if !actualTime.Equal(expectedTime) {
			t.Errorf("expected time %v, got %v", expectedTime, actualTime)
		}
	})
}
