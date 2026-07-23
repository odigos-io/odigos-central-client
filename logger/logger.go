package logger

import "context"

type CustomLogger interface {
	Info(ctx context.Context, msg string, args ...any)
	Debug(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, args ...any)
}

type LoggerConfig struct {
	Logger CustomLogger
}

type LoggingOption func(*LoggerConfig)

func WithLogger(logger CustomLogger) LoggingOption {
	return func(l *LoggerConfig) {
		l.Logger = logger
	}
}

func SetLogConfig(options []LoggingOption) LoggerConfig {

	logConfig := LoggerConfig{}
	for _, opt := range options {
		opt(&logConfig)
	}

	if logConfig.Logger == nil {
		logConfig.Logger = NewSlogLogger()
	}

	return logConfig
}
