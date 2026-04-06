package logging

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
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

// LogMetadata is a mutable metadata container shared between the access log
// middleware and the inner middleware/handlers via a pointer in context.
// Inner layers write to it; the access log reads it after the request finishes.
type LogMetadata struct {
	Action string
	// User is the authenticated principal (username or access key) or "anonymous".
	// Written by the auth middleware; empty for unauthenticated internal endpoints.
	User string
	// AuthzDecision is "allow" or "deny", written by the authorization middleware.
	// Empty for routes that bypass authorization (health, metrics, etc.).
	AuthzDecision string
	Custom        map[string]string
}

func GetLogData(ctx context.Context) (*LogMetadata, bool) {
	metadata, ok := ctx.Value(logDataKey).(*LogMetadata)
	if !ok {
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

			action := teapot.GetAction(r)
			if action == "" {
				action = data.Action
			}

			// Build log attributes
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"remote", r.RemoteAddr,
				"duration_ms", duration.Milliseconds(),
			}

			// Service classification — omit for internal routes with no action.
			if service := serviceFromAction(action); service != "" {
				attrs = append(attrs, "service", service)
			}

			// User identity — written by auth middleware; omit for internal endpoints.
			if data.User != "" {
				attrs = append(attrs, "user", data.User)
			}

			// S3 resource context.
			if bucket := teapot.URLParam(r, "bucket"); bucket != "" {
				attrs = append(attrs, "bucket", bucket)
			}
			if object := teapot.URLParam(r, "key"); object != "" {
				attrs = append(attrs, "object", object)
			}

			// Authorization decision — written by authz middleware; omit for bypass routes.
			if data.AuthzDecision != "" {
				attrs = append(attrs, "authz_decision", data.AuthzDecision)
			}

			// Route metadata.
			if action != "" {
				attrs = append(attrs, "action", action)
			}
			attrs = append(attrs,
				"route_name", teapot.GetRouteName(r),
				"req_time", start.Format(time.RFC3339Nano),
				"resp_time", end.Format(time.RFC3339Nano),
			)

			// Optional extras.
			if r.URL.RawQuery != "" {
				attrs = append(attrs, "query", r.URL.RawQuery)
			}
			if etag := wrapped.Header().Get("ETag"); etag != "" {
				attrs = append(attrs, "etag", etag)
			}
			if len(data.Custom) > 0 {
				attrs = append(attrs, "extra", data.Custom)
			}

			// Health, metrics, and favicon are hit constantly by browsers/Docker/orchestrators;
			// log them at DEBUG to avoid flooding logs under normal operation.
			if isHighFrequencyRoute(action, teapot.GetRouteName(r)) {
				log.Debug("http request handled", attrs...)
			} else {
				log.Info("http request handled", attrs...)
			}
		})
	}
}

// serviceFromAction extracts the service prefix from a route action string.
// e.g. "s3:GetObject" → "s3", "dirio:Health" → "dirio", "" → "".
func serviceFromAction(action string) string {
	service, _, _ := strings.Cut(action, ":")
	return service
}

// isHighFrequencyRoute reports whether the request is to a route that is hit at
// high frequency by browsers, Docker, orchestrators, or monitoring scrapers.
// These are logged at DEBUG to avoid flooding production logs.
func isHighFrequencyRoute(action, routeName string) bool {
	switch action {
	case "dirio:Health", "dirio:HealthReady", "dirio:HealthLive",
		"minio:HealthLive", "minio:HealthReady",
		"dirio:Metrics":
		return true
	}
	return routeName == "favicon"
}
