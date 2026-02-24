package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/http/trace"
)

func TestPrepareAccessLogMiddleware(t *testing.T) {
	t.Run("logs request with duration", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		// Add trace ID so we can verify context-aware logging
		ctx := trace.WithTraceID(req.Context(), "test-trace-123")
		req = req.WithContext(ctx)

		handler := PrepareAccessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate some work
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rec, req)

		// Parse log output
		var logEntry map[string]any
		if err := json.Unmarshal(logBuf.Bytes(), &logEntry); err != nil {
			t.Fatalf("failed to parse log output: %v", err)
		}

		// Verify basic fields
		if logEntry["method"] != "GET" {
			t.Errorf("expected method GET, got %v", logEntry["method"])
		}
		if logEntry["path"] != "/test" {
			t.Errorf("expected path /test, got %v", logEntry["path"])
		}
		if logEntry["status"] != float64(200) {
			t.Errorf("expected status 200, got %v", logEntry["status"])
		}

		// Verify duration is present and reasonable (>= 10ms since we slept)
		duration, ok := logEntry["duration_ms"].(float64)
		if !ok {
			t.Fatalf("expected duration_ms to be present and numeric, got %v", logEntry["duration_ms"])
		}
		if duration < 10 {
			t.Errorf("expected duration >= 10ms (we slept 10ms), got %vms", duration)
		}
	})

	t.Run("logs query parameters", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

		req := httptest.NewRequest("GET", "/bucket?list-type=2&prefix=test/", nil)
		rec := httptest.NewRecorder()

		handler := PrepareAccessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rec, req)

		// Parse log output
		var logEntry map[string]any
		if err := json.Unmarshal(logBuf.Bytes(), &logEntry); err != nil {
			t.Fatalf("failed to parse log output: %v", err)
		}

		// Verify query parameters are logged
		query, ok := logEntry["query"].(string)
		if !ok {
			t.Fatalf("expected query to be present as string, got %v", logEntry["query"])
		}
		if query != "list-type=2&prefix=test/" {
			t.Errorf("expected query 'list-type=2&prefix=test/', got %q", query)
		}
	})

	t.Run("does not log query when absent", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		handler := PrepareAccessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rec, req)

		// Parse log output
		var logEntry map[string]any
		if err := json.Unmarshal(logBuf.Bytes(), &logEntry); err != nil {
			t.Fatalf("failed to parse log output: %v", err)
		}

		// Verify query is not present
		if _, exists := logEntry["query"]; exists {
			t.Errorf("expected query to be absent, but it was present with value: %v", logEntry["query"])
		}
	})

	t.Run("logs operation when Action is set", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

		req := httptest.NewRequest("GET", "/", nil)
		rec := httptest.NewRecorder()

		handler := PrepareAccessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate handler setting the Action field
			if data, ok := GetLogData(r.Context()); ok {
				data.Action = "ListBuckets"
			}
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rec, req)

		// Parse log output
		var logEntry map[string]any
		if err := json.Unmarshal(logBuf.Bytes(), &logEntry); err != nil {
			t.Fatalf("failed to parse log output: %v", err)
		}

		// Verify operation is logged
		operation, ok := logEntry["operation"].(string)
		if !ok {
			t.Fatalf("expected operation to be present as string, got %v", logEntry["operation"])
		}
		if operation != "ListBuckets" {
			t.Errorf("expected operation 'ListBuckets', got %q", operation)
		}
	})

	t.Run("logs custom metadata", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		handler := PrepareAccessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate handler adding custom metadata
			if data, ok := GetLogData(r.Context()); ok {
				data.Custom["bucket"] = "my-bucket"
				data.Custom["key"] = "my-key"
			}
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rec, req)

		// Parse log output
		var logEntry map[string]any
		if err := json.Unmarshal(logBuf.Bytes(), &logEntry); err != nil {
			t.Fatalf("failed to parse log output: %v", err)
		}

		// Verify custom metadata is logged
		extra, ok := logEntry["extra"].(map[string]any)
		if !ok {
			t.Fatalf("expected extra to be present as map, got %v", logEntry["extra"])
		}
		if extra["bucket"] != "my-bucket" {
			t.Errorf("expected extra.bucket 'my-bucket', got %v", extra["bucket"])
		}
		if extra["key"] != "my-key" {
			t.Errorf("expected extra.key 'my-key', got %v", extra["key"])
		}
	})

	t.Run("uses timing middleware for accurate req_time", func(t *testing.T) {
		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		// Chain Timing middleware first, then logging middleware
		handler := middleware.Timing(PrepareAccessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate some work between middlewares
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})))

		beforeRequest := time.Now()
		handler.ServeHTTP(rec, req)
		afterRequest := time.Now()

		// Parse log output
		var logEntry map[string]any
		if err := json.Unmarshal(logBuf.Bytes(), &logEntry); err != nil {
			t.Fatalf("failed to parse log output: %v", err)
		}

		// Verify req_time is present
		reqTimeStr, ok := logEntry["req_time"].(string)
		if !ok {
			t.Fatalf("expected req_time to be present as string, got %v", logEntry["req_time"])
		}
		reqTime, err := time.Parse(time.RFC3339, reqTimeStr)
		if err != nil {
			t.Fatalf("failed to parse req_time: %v", err)
		}

		// Verify resp_time is present
		respTimeStr, ok := logEntry["resp_time"].(string)
		if !ok {
			t.Fatalf("expected resp_time to be present as string, got %v", logEntry["resp_time"])
		}
		respTime, err := time.Parse(time.RFC3339, respTimeStr)
		if err != nil {
			t.Fatalf("failed to parse resp_time: %v", err)
		}

		// Verify req_time is before or equal to resp_time (RFC3339 has second precision)
		if reqTime.After(respTime) {
			t.Errorf("expected req_time %v to be before or equal to resp_time %v", reqTime, respTime)
		}

		// Verify req_time is within reasonable bounds (allow for RFC3339 second-level precision)
		// beforeRequest and afterRequest are within a few milliseconds, so truncating to seconds is OK
		beforeTruncated := beforeRequest.Truncate(time.Second)
		afterTruncated := afterRequest.Truncate(time.Second).Add(time.Second) // Add 1 second for rounding
		if reqTime.Before(beforeTruncated) || reqTime.After(afterTruncated) {
			t.Errorf("req_time %v is not within reasonable range of request time", reqTime)
		}

		// Verify duration is at least 10ms (we slept for 10ms)
		duration, ok := logEntry["duration_ms"].(float64)
		if !ok {
			t.Fatalf("expected duration_ms to be present and numeric, got %v", logEntry["duration_ms"])
		}
		if duration < 10 {
			t.Errorf("expected duration >= 10ms (we slept 10ms), got %vms", duration)
		}
	})
}

func TestGetLogData(t *testing.T) {
	t.Run("returns nil for context without metadata", func(t *testing.T) {
		ctx := context.Background()
		data, ok := GetLogData(ctx)
		if ok {
			t.Error("expected ok to be false for context without metadata")
		}
		if data != nil {
			t.Errorf("expected nil data, got %v", data)
		}
	})

	t.Run("returns metadata when present", func(t *testing.T) {
		metadata := &LogMetadata{
			Action: "TestAction",
			Custom: map[string]string{"key": "value"},
		}
		ctx := context.WithValue(context.Background(), logDataKey, metadata)

		data, ok := GetLogData(ctx)
		if !ok {
			t.Error("expected ok to be true for context with metadata")
		}
		if data != metadata {
			t.Error("expected to get the same metadata instance")
		}
		if data.Action != "TestAction" {
			t.Errorf("expected Action 'TestAction', got %q", data.Action)
		}
	})
}
