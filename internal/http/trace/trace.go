package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	contextInt "github.com/mallardduck/dirio/internal/context"
)

const (
	// TraceIDHeader is the HTTP header name for trace IDs (X-Trace-ID for simplicity)
	TraceIDHeader = "X-Trace-ID"
)

// WithTraceID returns a new context with the given trace ID attached.
// Use this at entry points to create a new trace.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, contextInt.TraceIDKey, traceID)
}

// FromContext extracts the trace ID from the context.
// Returns "unknown" if no trace ID is found, making missing trace IDs obvious.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return "unknown"
	}
	if id, ok := ctx.Value(contextInt.TraceIDKey).(string); ok {
		return id
	}
	return "unknown"
}

// NewTraceID generates a new unique trace ID using crypto/rand.
// Format: 16 bytes of random data encoded as 32 character hex string.
func NewTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to a recognizable error value if random generation fails
		return "error-generating-trace-id"
	}
	return hex.EncodeToString(b)
}
