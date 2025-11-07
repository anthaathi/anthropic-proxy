package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

var log *slog.Logger
var tuiHandler slog.Handler

// Init initializes the global logger with the specified log level
func Init(levelStr string) {
	level := parseLogLevel(levelStr)

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	log = slog.New(handler)
}

// InitQuiet initializes the logger with output discarded (useful for TUI mode startup)
func InitQuiet(levelStr string) {
	level := parseLogLevel(levelStr)

	handler := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{
		Level: level,
	})

	log = slog.New(handler)
}

// InitWithTUI initializes the logger with a custom TUI handler
func InitWithTUI(levelStr string, customHandler slog.Handler) {
	tuiHandler = customHandler
	log = slog.New(customHandler)
}

// SetTUIHandler sets a TUI handler (useful for adding after initialization)
func SetTUIHandler(handler slog.Handler) {
	tuiHandler = handler
	log = slog.New(handler)
}

// parseLogLevel converts a string log level to slog.Level
func parseLogLevel(levelStr string) slog.Level {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo // Default to INFO
	}
}

// Debug logs a debug message with optional key-value pairs
func Debug(msg string, args ...any) {
	log.Debug(msg, args...)
}

// Info logs an info message with optional key-value pairs
func Info(msg string, args ...any) {
	log.Info(msg, args...)
}

// Warn logs a warning message with optional key-value pairs
func Warn(msg string, args ...any) {
	log.Warn(msg, args...)
}

// Error logs an error message with optional key-value pairs
func Error(msg string, args ...any) {
	log.Error(msg, args...)
}

// With returns a new logger with the given key-value pairs added as context
func With(args ...any) *slog.Logger {
	return log.With(args...)
}

// GetLogger returns the underlying slog.Logger
func GetLogger() *slog.Logger {
	return log
}
