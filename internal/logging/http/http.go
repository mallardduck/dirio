package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/logging"
)

// contextKey for storing response writer in context
type contextKey string

const (
	logDataKey contextKey = "logData"
)

// LogMetadata is a mutable metadata container for logging
type LogMetadata struct {
	Action string
	Custom map[string]string
}

func GetLogData(ctx context.Context) (*LogMetadata, bool) {
	var metadata *LogMetadata
	var ok bool
	if metadata, ok = ctx.Value(logDataKey).(*LogMetadata); !ok {
		return nil, false
	}

	return metadata, true
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// PrepareAccessLogMiddleware builds a logging middleware instance using the provided logger
func PrepareAccessLogMiddleware(serverLogger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try to get the request start time from context (set by Timing middleware)
			// If not found, capture it now as a fallback
			var start time.Time
			if reqStart, ok := middleware.GetRequestStartTime(r.Context()); ok {
				start = reqStart
			} else {
				start = time.Now()
			}

			// Initialize the metadata container
			data := &LogMetadata{Custom: make(map[string]string)}

			// Wrap response writer to capture status code and operation
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Make the wrapped writer available to handlers through context
			ctx := context.WithValue(r.Context(), logDataKey, data)
			r = r.WithContext(ctx)

			// Pass the new context down the chain
			next.ServeHTTP(wrapped, r)

			// Capture response time and calculate duration
			end := time.Now()
			duration := end.Sub(start)

			// Use context-aware logging (automatically includes trace_id)
			log := logging.WithContext(serverLogger, r.Context())

			// Build log attributes
			attrs := []any{
				"route_name", teapot.GetRouteName(r),
				"route_action", teapot.GetAction(r),
				"method", r.Method,
				"host", r.URL.Host,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"remote", r.RemoteAddr,
				"req_time", start.Format(time.RFC3339Nano),
				"resp_time", end.Format(time.RFC3339Nano),
				"duration_ms", duration.Milliseconds(),
			}

			// Add query parameters if present
			if r.URL.RawQuery != "" {
				attrs = append(attrs, "query", r.URL.RawQuery)
			}

			// Add fragment if present
			if r.URL.Fragment != "" {
				attrs = append(attrs, "fragment", r.URL.Fragment)
			}

			// Add operation name if present
			if data.Action != "" {
				attrs = append(attrs, "operation", data.Action)
			}
			if len(data.Custom) > 0 {
				attrs = append(attrs, "extra", data.Custom)
			}

			log.Info("http request handled", attrs...)
		})
	}
}
