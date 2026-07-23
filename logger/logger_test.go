package logger

import (
	"context"
	"testing"
)

// mockLogger is a test double that records all log calls.
type mockLogger struct {
	infoCalls  []logCall
	debugCalls []logCall
	errorCalls []logCall
}

type logCall struct {
	msg  string
	args []any
}

func (m *mockLogger) Info(_ context.Context, msg string, args ...any) {
	m.infoCalls = append(m.infoCalls, logCall{msg: msg, args: args})
}

func (m *mockLogger) Debug(_ context.Context, msg string, args ...any) {
	m.debugCalls = append(m.debugCalls, logCall{msg: msg, args: args})
}

func (m *mockLogger) Error(_ context.Context, msg string, args ...any) {
	m.errorCalls = append(m.errorCalls, logCall{msg: msg, args: args})
}

// ---------------------------------------------------------------------------
// CustomLogger interface compliance
// ---------------------------------------------------------------------------

func TestMockLogger_ImplementsCustomLogger(t *testing.T) {
	var _ CustomLogger = (*mockLogger)(nil)
}

func TestSlogLogger_ImplementsCustomLogger(t *testing.T) {
	var _ CustomLogger = (*SlogLogger)(nil)
}

// ---------------------------------------------------------------------------
// WithLogger option
// ---------------------------------------------------------------------------

func TestWithLogger(t *testing.T) {
	ml := &mockLogger{}
	opt := WithLogger(ml)

	cfg := &LoggerConfig{}
	opt(cfg)

	if cfg.Logger != ml {
		t.Error("WithLogger did not set the logger")
	}
}

func TestWithLogger_NilLogger(t *testing.T) {
	opt := WithLogger(nil)

	cfg := &LoggerConfig{}
	opt(cfg)

	if cfg.Logger != nil {
		t.Error("expected nil logger")
	}
}

// ---------------------------------------------------------------------------
// SetLogConfig tests
// ---------------------------------------------------------------------------

func TestSetLogConfig_NoOptions(t *testing.T) {
	cfg := SetLogConfig(nil)

	if cfg.Logger == nil {
		t.Fatal("expected default logger when no options provided")
	}
	// Default logger should be a SlogLogger
	if _, ok := cfg.Logger.(*SlogLogger); !ok {
		t.Errorf("expected *SlogLogger, got %T", cfg.Logger)
	}
}

func TestSetLogConfig_EmptySlice(t *testing.T) {
	cfg := SetLogConfig([]LoggingOption{})

	if cfg.Logger == nil {
		t.Fatal("expected default logger for empty options slice")
	}
	if _, ok := cfg.Logger.(*SlogLogger); !ok {
		t.Errorf("expected *SlogLogger, got %T", cfg.Logger)
	}
}

func TestSetLogConfig_WithCustomLogger(t *testing.T) {
	ml := &mockLogger{}
	cfg := SetLogConfig([]LoggingOption{WithLogger(ml)})

	if cfg.Logger != ml {
		t.Error("expected custom logger to be set")
	}
}

func TestSetLogConfig_OptionOrderMatters(t *testing.T) {
	ml1 := &mockLogger{}
	ml2 := &mockLogger{}
	cfg := SetLogConfig([]LoggingOption{
		WithLogger(ml1),
		WithLogger(ml2), // should override ml1
	})

	if cfg.Logger != ml2 {
		t.Error("expected last logger option to win")
	}
}

func TestSetLogConfig_NilLoggerOptionFallsBackToDefault(t *testing.T) {
	// Passing WithLogger(nil) explicitly should trigger the default fallback
	cfg := SetLogConfig([]LoggingOption{WithLogger(nil)})

	if cfg.Logger == nil {
		t.Fatal("expected default logger when nil is passed")
	}
	if _, ok := cfg.Logger.(*SlogLogger); !ok {
		t.Errorf("expected *SlogLogger fallback, got %T", cfg.Logger)
	}
}

// ---------------------------------------------------------------------------
// LoggerConfig struct field access
// ---------------------------------------------------------------------------

func TestLoggerConfig_UsableAfterSetup(t *testing.T) {
	ml := &mockLogger{}
	cfg := SetLogConfig([]LoggingOption{
		WithLogger(ml),
	})

	// Use the logger through the config
	cfg.Logger.Info(context.Background(), "test message", "key", "value")
	cfg.Logger.Debug(context.Background(), "debug msg")
	cfg.Logger.Error(context.Background(), "error msg", "code", 500)

	if len(ml.infoCalls) != 1 {
		t.Fatalf("expected 1 info call, got %d", len(ml.infoCalls))
	}
	if ml.infoCalls[0].msg != "test message" {
		t.Errorf("expected msg='test message', got %q", ml.infoCalls[0].msg)
	}
	if len(ml.infoCalls[0].args) != 2 {
		t.Errorf("expected 2 args, got %d", len(ml.infoCalls[0].args))
	}

	if len(ml.debugCalls) != 1 {
		t.Fatalf("expected 1 debug call, got %d", len(ml.debugCalls))
	}
	if ml.debugCalls[0].msg != "debug msg" {
		t.Errorf("expected msg='debug msg', got %q", ml.debugCalls[0].msg)
	}

	if len(ml.errorCalls) != 1 {
		t.Fatalf("expected 1 error call, got %d", len(ml.errorCalls))
	}
	if ml.errorCalls[0].msg != "error msg" {
		t.Errorf("expected msg='error msg', got %q", ml.errorCalls[0].msg)
	}
}

// ---------------------------------------------------------------------------
// LoggingOption type
// ---------------------------------------------------------------------------

func TestLoggingOption_IsFunction(t *testing.T) {
	// Verify LoggingOption works as a function type
	var opt LoggingOption = func(cfg *LoggerConfig) {
		cfg.Logger = &mockLogger{}
	}

	cfg := &LoggerConfig{}
	opt(cfg)

	if cfg.Logger == nil {
		t.Error("custom LoggingOption should have set the logger")
	}
}
