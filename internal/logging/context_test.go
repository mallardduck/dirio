package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/mallardduck/dirio/internal/http/trace"
)

func TestFromContext(t *testing.T) {
	// Setup a test logger that writes to a buffer
	var buf bytes.Buffer
	Setup(Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	})

	t.Run("includes trace_id from context", func(t *testing.T) {
		buf.Reset()
		traceID := "test-trace-123"
		ctx := trace.WithTraceID(context.Background(), traceID)

		log := FromContext(ctx)
		log.Info("test message")

		output := buf.String()
		if !strings.Contains(output, "trace_id="+traceID) {
			t.Errorf("expected log to contain trace_id=%s, got: %s", traceID, output)
		}
		if !strings.Contains(output, "test message") {
			t.Errorf("expected log to contain message, got: %s", output)
		}
	})

	t.Run("includes unknown when no trace_id in context", func(t *testing.T) {
		buf.Reset()
		ctx := context.Background()

		log := FromContext(ctx)
		log.Info("test message")

		output := buf.String()
		if !strings.Contains(output, "trace_id=unknown") {
			t.Errorf("expected log to contain trace_id=unknown, got: %s", output)
		}
	})

	t.Run("preserves additional attributes", func(t *testing.T) {
		buf.Reset()
		traceID := "test-trace-456"
		ctx := trace.WithTraceID(context.Background(), traceID)

		log := FromContext(ctx)
		log.Info("test message", "user", "alice", "count", 42)

		output := buf.String()
		if !strings.Contains(output, "trace_id="+traceID) {
			t.Error("expected trace_id in output")
		}
		if !strings.Contains(output, "user=alice") {
			t.Error("expected user attribute in output")
		}
		if !strings.Contains(output, "count=42") {
			t.Error("expected count attribute in output")
		}
	})
}

func TestComponentWithContext(t *testing.T) {
	var buf bytes.Buffer
	Setup(Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	})

	t.Run("includes both component and trace_id", func(t *testing.T) {
		buf.Reset()
		traceID := "test-trace-789"
		ctx := trace.WithTraceID(context.Background(), traceID)

		log := ComponentWithContext(ctx, "storage")
		log.Info("test message")

		output := buf.String()
		if !strings.Contains(output, "component=storage") {
			t.Errorf("expected log to contain component=storage, got: %s", output)
		}
		if !strings.Contains(output, "trace_id="+traceID) {
			t.Errorf("expected log to contain trace_id=%s, got: %s", traceID, output)
		}
	})

	t.Run("includes unknown trace_id when not in context", func(t *testing.T) {
		buf.Reset()
		ctx := context.Background()

		log := ComponentWithContext(ctx, "api")
		log.Info("test message")

		output := buf.String()
		if !strings.Contains(output, "component=api") {
			t.Error("expected component in output")
		}
		if !strings.Contains(output, "trace_id=unknown") {
			t.Error("expected trace_id=unknown in output")
		}
	})
}

func TestWithContext(t *testing.T) {
	var buf bytes.Buffer
	Setup(Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	})

	t.Run("adds trace_id to existing logger", func(t *testing.T) {
		buf.Reset()
		traceID := "test-trace-999"
		ctx := trace.WithTraceID(context.Background(), traceID)

		// Create a logger with some existing attributes
		baseLogger := slog.New(slog.NewTextHandler(&buf, nil)).With("service", "dirio")
		log := WithContext(baseLogger, ctx)
		log.Info("test message")

		output := buf.String()
		if !strings.Contains(output, "service=dirio") {
			t.Error("expected existing service attribute in output")
		}
		if !strings.Contains(output, "trace_id="+traceID) {
			t.Errorf("expected trace_id=%s in output, got: %s", traceID, output)
		}
	})

	t.Run("works with Component logger", func(t *testing.T) {
		buf.Reset()
		traceID := "test-trace-component"
		ctx := trace.WithTraceID(context.Background(), traceID)

		// Start with a component logger, then add context
		log := WithContext(Component("server"), ctx)
		log.Info("test message")

		output := buf.String()
		if !strings.Contains(output, "component=server") {
			t.Error("expected component in output")
		}
		if !strings.Contains(output, "trace_id="+traceID) {
			t.Error("expected trace_id in output")
		}
	})
}

func TestContextLoggingPatterns(t *testing.T) {
	var buf bytes.Buffer
	Setup(Config{
		Level:  "info",
		Format: "text",
		Output: &buf,
	})

	t.Run("recommended pattern: FromContext for simple cases", func(t *testing.T) {
		buf.Reset()
		ctx := trace.WithTraceID(context.Background(), "simple-trace")

		// This is the simplest pattern
		log := FromContext(ctx)
		log.Info("operation completed")

		output := buf.String()
		if !strings.Contains(output, "trace_id=simple-trace") {
			t.Error("expected trace_id in output")
		}
	})

	t.Run("recommended pattern: ComponentWithContext for components", func(t *testing.T) {
		buf.Reset()
		ctx := trace.WithTraceID(context.Background(), "component-trace")

		// This is the pattern for component-specific loggers
		log := ComponentWithContext(ctx, "storage")
		log.Info("bucket created")

		output := buf.String()
		if !strings.Contains(output, "component=storage") {
			t.Error("expected component in output")
		}
		if !strings.Contains(output, "trace_id=component-trace") {
			t.Error("expected trace_id in output")
		}
	})
}
