package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const (
	// RequestIDKey is the context key for request IDs
	RequestIDKey contextKey = "requestID"
	// RequestIDHeader is the HTTP header name for request IDs
	RequestIDHeader = "X-Request-Id"
)

// RequestID is a middleware that generates a unique request ID for each request.
// It adds the request ID to the request context and optionally to the response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request already has an ID from upstream proxy
		requestID := r.Header.Get(RequestIDHeader)
		if requestID == "" {
			// Generate a new request ID
			requestID = generateRequestID()
		}

		// Add request ID to response headers
		w.Header().Set(RequestIDHeader, requestID)

		// Add request ID to context
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extracts the request ID from the context.
// Returns an empty string if no request ID is found.
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// generateRequestID creates a new unique request ID using crypto/rand.
// Format: 16 bytes of random data encoded as 32 character hex string.
func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to a simple ID if random generation fails
		return "error-generating-id"
	}
	return hex.EncodeToString(b)
}