package http

import (
	"context"
	"log/slog"
	"net/http"

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

// PrepareLoggingMiddleware builds a logging middleware instance using the provided logger
func PrepareLoggingMiddleware(serverLogger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Initialize the metadata container
			data := &LogMetadata{Custom: make(map[string]string)}

			// Wrap response writer to capture status code and operation
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Make the wrapped writer available to handlers through context
			ctx := context.WithValue(r.Context(), logDataKey, data)

			// Pass the new context down the chain
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Use context-aware logging (automatically includes trace_id)
			log := logging.WithContext(serverLogger, r.Context())

			// Build log attributes
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"remote", r.RemoteAddr,
			}

			// Add operation name if present
			if data.Action != "" {
				attrs = append(attrs, "operation", data.Action)
			}
			if data.Custom != nil && len(data.Custom) > 0 {
				attrs = append(attrs, "extra", data.Custom)
			}

			log.Info("http request handled", attrs...)
		})
	}
}
