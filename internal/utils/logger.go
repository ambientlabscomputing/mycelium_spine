package utils

import (
	"context"
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

var Logger *slog.Logger

type LogContextKey struct{}

func InitLogger(settings *Settings) {
	var level slog.Level
	switch settings.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// build writer -- to stderr, file, or both, use MultiWriter
	var writers []io.Writer
	if settings.LogToStderr {
		writers = append(writers, os.Stderr)
	}
	if settings.LogToFile {
		lumberjackLogger := &lumberjack.Logger{
			Filename:   settings.LogFilePath,
			MaxSize:    10, // megabytes
			MaxBackups: 5,
			MaxAge:     28,   //days
			Compress:   true, // disabled by default
		}
		writers = append(writers, lumberjackLogger)
	}

	multiWriter := io.MultiWriter(writers...)

	// build handler
	var handler slog.Handler
	if settings.LogFormat == "json" {
		handler = slog.NewJSONHandler(multiWriter, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(multiWriter, &slog.HandlerOptions{Level: level})
	}

	// create logger
	Logger = slog.New(handler)
	Logger.Info("Logger initialized", "level", settings.LogLevel, "format", settings.LogFormat)
	slog.SetDefault(Logger)
}

func InitLoggerWithContext(ctx context.Context, settings *Settings) (*slog.Logger, context.Context) {
	InitLogger(settings)
	ctx = context.WithValue(ctx, LogContextKey{}, Logger)
	return Logger, ctx
}

func GetLogger(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return Logger
	}
	if logger, ok := ctx.Value(LogContextKey{}).(*slog.Logger); ok {
		return logger
	}
	return Logger
}

func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, LogContextKey{}, logger)
}
