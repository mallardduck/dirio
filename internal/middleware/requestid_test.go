package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestID(t *testing.T) {
	t.Run("generates request ID when not present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		var capturedID string
		handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = GetRequestID(r.Context())
		}))

		handler.ServeHTTP(rec, req)

		// Verify request ID was generated
		if capturedID == "" {
			t.Error("expected request ID to be generated, got empty string")
		}

		// Verify request ID is in response headers
		responseID := rec.Header().Get(RequestIDHeader)
		if responseID != capturedID {
			t.Errorf("expected response header %q, got %q", capturedID, responseID)
		}

		// Verify request ID is 32 characters (16 bytes in hex)
		if len(capturedID) != 32 {
			t.Errorf("expected request ID length 32, got %d", len(capturedID))
		}
	})

	t.Run("uses existing request ID from header", func(t *testing.T) {
		existingID := "existing-request-id-123"
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set(RequestIDHeader, existingID)
		rec := httptest.NewRecorder()

		var capturedID string
		handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = GetRequestID(r.Context())
		}))

		handler.ServeHTTP(rec, req)

		// Verify existing request ID was used
		if capturedID != existingID {
			t.Errorf("expected request ID %q, got %q", existingID, capturedID)
		}

		// Verify request ID is in response headers
		responseID := rec.Header().Get(RequestIDHeader)
		if responseID != existingID {
			t.Errorf("expected response header %q, got %q", existingID, responseID)
		}
	})

	t.Run("GetRequestID returns empty string for nil context", func(t *testing.T) {
		id := GetRequestID(nil)
		if id != "" {
			t.Errorf("expected empty string for nil context, got %q", id)
		}
	})

	t.Run("GetRequestID returns empty string when no ID in context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		id := GetRequestID(req.Context())
		if id != "" {
			t.Errorf("expected empty string for context without ID, got %q", id)
		}
	})
}