package middleware

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/trace"
)

// TraceID is a middleware that generates or accepts a trace ID for each request.
// This is the first middleware that should run, creating the trace context for the entire request lifecycle.
// It checks for an incoming X-Trace-ID header and uses it if present, otherwise generates a new one.
func TraceID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request already has a trace ID from upstream
		traceID := r.Header.Get(trace.TraceIDHeader)
		if traceID == "" {
			// Generate a new trace ID
			traceID = trace.NewTraceID()
		}

		// Add trace ID to response headers
		w.Header().Set(trace.TraceIDHeader, traceID)

		// Add trace ID to context
		ctx := trace.WithTraceID(r.Context(), traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
