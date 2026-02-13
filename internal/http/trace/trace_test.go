package trace

import (
	"context"
	"testing"
)

func TestNewTraceID(t *testing.T) {
	t.Run("generates unique trace IDs", func(t *testing.T) {
		id1 := NewTraceID()
		id2 := NewTraceID()

		if id1 == id2 {
			t.Error("expected unique trace IDs, got duplicates")
		}

		if len(id1) != 32 {
			t.Errorf("expected trace ID length 32, got %d", len(id1))
		}
	})
}

func TestWithTraceID(t *testing.T) {
	t.Run("stores and retrieves trace ID", func(t *testing.T) {
		traceID := "test-trace-123"
		ctx := WithTraceID(context.Background(), traceID)

		retrieved := FromContext(ctx)
		if retrieved != traceID {
			t.Errorf("expected trace ID %q, got %q", traceID, retrieved)
		}
	})
}

func TestFromContext(t *testing.T) {
	t.Run("returns unknown for nil context", func(t *testing.T) {
		id := FromContext(context.Background())
		if id != "unknown" {
			t.Errorf("expected 'unknown' for nil context, got %q", id)
		}
	})

	t.Run("returns unknown for context without trace ID", func(t *testing.T) {
		ctx := context.Background()
		id := FromContext(ctx)
		if id != "unknown" {
			t.Errorf("expected 'unknown' for context without trace ID, got %q", id)
		}
	})

	t.Run("returns trace ID from context", func(t *testing.T) {
		traceID := "abc123def456"
		ctx := WithTraceID(context.Background(), traceID)
		id := FromContext(ctx)
		if id != traceID {
			t.Errorf("expected %q, got %q", traceID, id)
		}
	})
}
