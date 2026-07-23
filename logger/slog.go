package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type SlogLogger struct {
	logger *slog.Logger
}

func NewSlogLogger() *SlogLogger {
	var level = new(slog.LevelVar)
	logLevel := strings.ToUpper(os.Getenv("ODIGOS_LOG"))
	switch logLevel {
	case "DEBUG":
		level.Set(slog.LevelDebug)
	case "INFO":
		level.Set(slog.LevelInfo)
	case "WARN":
		level.Set(slog.LevelWarn)
	case "ERROR":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})

	return &SlogLogger{
		logger: slog.New(handler),
	}
}

func (l *SlogLogger) Info(ctx context.Context, msg string, args ...any) {
	l.logger.InfoContext(ctx, msg, args...)
}

func (l *SlogLogger) Debug(ctx context.Context, msg string, args ...any) {
	l.logger.DebugContext(ctx, msg, args...)
}

func (l *SlogLogger) Error(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, args...)
}
