package logger

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// NewSlogLogger tests
// ---------------------------------------------------------------------------

func TestNewSlogLogger_ReturnsNonNil(t *testing.T) {
	l := NewSlogLogger()
	if l == nil {
		t.Fatal("expected non-nil SlogLogger")
	}
	if l.logger == nil {
		t.Fatal("expected non-nil inner slog.Logger")
	}
}

func TestNewSlogLogger_LogLevels(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
	}{
		{"debug level", "DEBUG"},
		{"debug lowercase", "debug"},
		{"debug mixed case", "DeBuG"},
		{"info level", "INFO"},
		{"info lowercase", "info"},
		{"warn level", "WARN"},
		{"warn lowercase", "warn"},
		{"error level", "ERROR"},
		{"error lowercase", "error"},
		{"default for empty", ""},
		{"default for unknown", "TRACE"},
		{"default for garbage", "not-a-level"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ODIGOS_LOG", tt.envValue)
			l := NewSlogLogger()
			if l == nil {
				t.Fatal("expected non-nil SlogLogger")
			}
			if l.logger == nil {
				t.Fatal("expected non-nil inner logger")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SlogLogger method tests (ensure no panics, correct delegation)
// ---------------------------------------------------------------------------

func TestSlogLogger_Info(t *testing.T) {
	t.Setenv("ODIGOS_LOG", "INFO")
	l := NewSlogLogger()

	// Should not panic
	l.Info(context.Background(), "info message")
	l.Info(context.Background(), "info with args", "key", "value")
	l.Info(context.Background(), "info with multiple args", "k1", "v1", "k2", 42)
}

func TestSlogLogger_Debug(t *testing.T) {
	t.Setenv("ODIGOS_LOG", "DEBUG")
	l := NewSlogLogger()

	l.Debug(context.Background(), "debug message")
	l.Debug(context.Background(), "debug with args", "key", "value")
	l.Debug(context.Background(), "debug with multiple args", "k1", "v1", "k2", true)
}

func TestSlogLogger_Error(t *testing.T) {
	t.Setenv("ODIGOS_LOG", "ERROR")
	l := NewSlogLogger()

	l.Error(context.Background(), "error message")
	l.Error(context.Background(), "error with args", "err", "something failed")
	l.Error(context.Background(), "error with multiple args", "code", 500, "reason", "internal")
}

func TestSlogLogger_InfoNoArgs(t *testing.T) {
	l := NewSlogLogger()
	l.Info(context.Background(), "no extra args")
}

func TestSlogLogger_DebugNoArgs(t *testing.T) {
	t.Setenv("ODIGOS_LOG", "DEBUG")
	l := NewSlogLogger()
	l.Debug(context.Background(), "no extra args")
}

func TestSlogLogger_ErrorNoArgs(t *testing.T) {
	l := NewSlogLogger()
	l.Error(context.Background(), "no extra args")
}

func TestSlogLogger_EmptyMessage(t *testing.T) {
	l := NewSlogLogger()
	// Empty messages should not panic
	l.Info(context.Background(), "")
	l.Debug(context.Background(), "")
	l.Error(context.Background(), "")
}

type ctxKey string

func TestSlogLogger_WithCustomContext(t *testing.T) {
	l := NewSlogLogger()
	ctx := context.WithValue(context.Background(), ctxKey("traceID"), "abc-123")

	// Should pass context through without panicking
	l.Info(ctx, "with trace context", "traceID", "abc-123")
	l.Debug(ctx, "debug with context")
	l.Error(ctx, "error with context")
}

func TestSlogLogger_ManyArgs(t *testing.T) {
	l := NewSlogLogger()
	// Verify it handles many key-value pairs
	l.Info(context.Background(), "many args",
		"a", 1, "b", 2, "c", 3, "d", 4, "e", 5,
		"f", 6, "g", 7, "h", 8, "i", 9, "j", 10,
	)
}

// ---------------------------------------------------------------------------
// Environment variable edge cases
// ---------------------------------------------------------------------------

func TestNewSlogLogger_UnsetEnvVar(t *testing.T) {
	t.Setenv("ODIGOS_LOG", "")
	l := NewSlogLogger()
	if l == nil {
		t.Fatal("expected non-nil logger with unset env var")
	}
}

func TestNewSlogLogger_WhitespaceEnvVar(t *testing.T) {
	t.Setenv("ODIGOS_LOG", "  ")
	l := NewSlogLogger()
	if l == nil {
		t.Fatal("expected non-nil logger with whitespace env var")
	}
	// Should fall through to default (INFO) since "  " uppercased doesn't match any case
}

func TestNewSlogLogger_MultipleInstances(t *testing.T) {
	t.Setenv("ODIGOS_LOG", "DEBUG")
	l1 := NewSlogLogger()
	t.Setenv("ODIGOS_LOG", "ERROR")
	l2 := NewSlogLogger()

	if l1 == nil || l2 == nil {
		t.Fatal("expected non-nil loggers")
	}
	// They should be distinct instances
	if l1 == l2 {
		t.Error("expected different SlogLogger instances")
	}
}
