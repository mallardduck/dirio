package middleware

import (
	"context"
	"net/http"
	"time"
)

// contextKey for storing timing information
type timingContextKey string

const (
	// RequestStartTimeKey is the context key for the request start timestamp
	RequestStartTimeKey timingContextKey = "requestStartTime"
)

// Timing is a middleware that captures the request start time as early as possible.
// This should be the FIRST middleware in the chain to get the most accurate timestamp
// of when the server received the request.
func Timing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture the request start time immediately
		startTime := time.Now()

		// Add start time to context
		ctx := context.WithValue(r.Context(), RequestStartTimeKey, startTime)

		// Pass request with updated context to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestStartTime retrieves the request start time from context.
// Returns the time and true if found, or zero time and false if not found.
func GetRequestStartTime(ctx context.Context) (time.Time, bool) {
	if ctx == nil {
		return time.Time{}, false
	}
	if startTime, ok := ctx.Value(RequestStartTimeKey).(time.Time); ok {
		return startTime, true
	}
	return time.Time{}, false
}
