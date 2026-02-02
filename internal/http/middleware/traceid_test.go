package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mallardduck/dirio/internal/http/trace"
)

func TestTraceID(t *testing.T) {
	t.Run("generates trace ID when not present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		var capturedID string
		handler := TraceID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = trace.FromContext(r.Context())
		}))

		handler.ServeHTTP(rec, req)

		// Verify trace ID was generated
		if capturedID == "" || capturedID == "unknown" {
			t.Errorf("expected trace ID to be generated, got %q", capturedID)
		}

		// Verify trace ID is in response headers
		responseID := rec.Header().Get(trace.TraceIDHeader)
		if responseID != capturedID {
			t.Errorf("expected response header %q, got %q", capturedID, responseID)
		}

		// Verify trace ID is 32 characters (16 bytes in hex)
		if len(capturedID) != 32 {
			t.Errorf("expected trace ID length 32, got %d", len(capturedID))
		}
	})

	t.Run("uses existing trace ID from header", func(t *testing.T) {
		existingID := "existing-trace-id-456"
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(trace.TraceIDHeader, existingID)
		rec := httptest.NewRecorder()

		var capturedID string
		handler := TraceID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = trace.FromContext(r.Context())
		}))

		handler.ServeHTTP(rec, req)

		// Verify existing trace ID was used
		if capturedID != existingID {
			t.Errorf("expected trace ID %q, got %q", existingID, capturedID)
		}

		// Verify trace ID is in response headers
		responseID := rec.Header().Get(trace.TraceIDHeader)
		if responseID != existingID {
			t.Errorf("expected response header %q, got %q", existingID, responseID)
		}
	})

	t.Run("trace ID and request ID work together", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		var capturedTraceID string
		var capturedRequestID string

		// Chain both middlewares: TraceID then RequestID
		handler := TraceID(RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedTraceID = trace.FromContext(r.Context())
			capturedRequestID = GetRequestID(r.Context())
		})))

		handler.ServeHTTP(rec, req)

		// Both should be generated
		if capturedTraceID == "" || capturedTraceID == "unknown" {
			t.Errorf("expected trace ID to be generated, got %q", capturedTraceID)
		}
		if capturedRequestID == "" {
			t.Errorf("expected request ID to be generated, got %q", capturedRequestID)
		}

		// They should be present in response headers
		if rec.Header().Get(trace.TraceIDHeader) != capturedTraceID {
			t.Error("trace ID not in response headers")
		}
		if rec.Header().Get(RequestIDHeader) != capturedRequestID {
			t.Error("request ID not in response headers")
		}
	})
}
