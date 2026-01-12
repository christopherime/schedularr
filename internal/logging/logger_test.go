package logging

import (
	"log/slog"
	"testing"
)

func TestNewLogger_DefaultLevel(t *testing.T) {
	logger := NewLogger("", "text")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	// Default level should be info
	if !logger.Enabled(nil, slog.LevelInfo) {
		t.Error("Expected logger to be enabled at info level")
	}
}

func TestNewLogger_DebugLevel(t *testing.T) {
	logger := NewLogger("debug", "text")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	if !logger.Enabled(nil, slog.LevelDebug) {
		t.Error("Expected logger to be enabled at debug level")
	}
}

func TestNewLogger_InfoLevel(t *testing.T) {
	logger := NewLogger("info", "text")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	if !logger.Enabled(nil, slog.LevelInfo) {
		t.Error("Expected logger to be enabled at info level")
	}

	if logger.Enabled(nil, slog.LevelDebug) {
		t.Error("Expected logger to be disabled at debug level")
	}
}

func TestNewLogger_WarnLevel(t *testing.T) {
	logger := NewLogger("warn", "text")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	if !logger.Enabled(nil, slog.LevelWarn) {
		t.Error("Expected logger to be enabled at warn level")
	}

	if logger.Enabled(nil, slog.LevelInfo) {
		t.Error("Expected logger to be disabled at info level")
	}
}

func TestNewLogger_WarningLevel(t *testing.T) {
	logger := NewLogger("warning", "text")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	if !logger.Enabled(nil, slog.LevelWarn) {
		t.Error("Expected logger to be enabled at warn level")
	}
}

func TestNewLogger_ErrorLevel(t *testing.T) {
	logger := NewLogger("error", "text")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	if !logger.Enabled(nil, slog.LevelError) {
		t.Error("Expected logger to be enabled at error level")
	}

	if logger.Enabled(nil, slog.LevelWarn) {
		t.Error("Expected logger to be disabled at warn level")
	}
}

func TestNewLogger_JSONFormat(t *testing.T) {
	logger := NewLogger("info", "json")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	// Just verify it doesn't crash - we can't easily test the output format
	logger.Info("test message")
}

func TestNewLogger_TextFormat(t *testing.T) {
	logger := NewLogger("info", "text")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	// Just verify it doesn't crash
	logger.Info("test message")
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	logger := NewLogger("invalid", "text")
	if logger == nil {
		t.Fatal("NewLogger returned nil")
	}

	// Should default to info level
	if !logger.Enabled(nil, slog.LevelInfo) {
		t.Error("Expected logger to default to info level for invalid level")
	}
}

func TestNewLogger_CaseInsensitive(t *testing.T) {
	tests := []struct {
		level    string
		expected slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"Info", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"Error", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			logger := NewLogger(tt.level, "text")
			if !logger.Enabled(nil, tt.expected) {
				t.Errorf("Expected logger to be enabled at %v level", tt.expected)
			}
		})
	}
}

func TestSetDefault(t *testing.T) {
	logger := NewLogger("debug", "text")
	SetDefault(logger)

	// Verify the default logger was set
	defaultLogger := slog.Default()
	if defaultLogger == nil {
		t.Error("Default logger is nil after SetDefault")
	}
}

