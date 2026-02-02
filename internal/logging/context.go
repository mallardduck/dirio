package logging

import (
	"context"
	"log/slog"

	"github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/http/trace"
)

// FromContext returns a logger with the trace_id automatically included from context.
// This is the recommended way to get a logger - it ensures trace_id is never forgotten.
//
// Usage:
//
//	log := logging.FromContext(ctx)
//	log.Info("operation completed", "details", "some info")
//
// The trace_id will be automatically included in the log output.
func FromContext(ctx context.Context) *slog.Logger {
	traceID := trace.FromContext(ctx)
	return Default().With("trace_id", traceID)
}

// ComponentWithContext returns a logger with both component and trace_id attributes.
// This is the recommended way to create component-specific loggers.
//
// Usage:
//
//	log := logging.ComponentWithContext(ctx, "storage")
//	log.Info("bucket created", "bucket", "my-bucket")
//
// The log will include both component="storage" and trace_id from context.
func ComponentWithContext(ctx context.Context, component string) *slog.Logger {
	traceID := trace.FromContext(ctx)
	return Default().With("component", component, "trace_id", traceID)
}

// WithContext adds trace_id to an existing logger.
// Use this when you already have a logger and need to add trace context.
//
// Usage:
//
//	log := logging.Component("server").WithContext(ctx)
//	log.Info("request processed")
func WithContext(logger *slog.Logger, ctx context.Context) *slog.Logger {
	traceID := trace.FromContext(ctx)
	requestId := middleware.GetRequestID(ctx)
	if requestId != "" {
		return logger.With("trace_id", traceID, "request_id", requestId)
	}

	return logger.With("trace_id", traceID)
}
