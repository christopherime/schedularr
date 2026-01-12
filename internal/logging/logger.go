// Package logging provides structured logging configuration for Schedularr.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a new slog.Logger based on the provided configuration.
// Format can be "json" or "text", level can be "debug", "info", "warn", or "error".
func NewLogger(level, format string) *slog.Logger {
	// Parse log level
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	// Create handler options
	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	// Create handler based on format
	var handler slog.Handler
	if strings.ToLower(format) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// SetDefault sets the default logger for the application.
func SetDefault(logger *slog.Logger) {
	slog.SetDefault(logger)
}
