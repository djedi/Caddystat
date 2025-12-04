package logging

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"DEBUG", LevelDebug},
		{"debug", LevelDebug},
		{"  DEBUG  ", LevelDebug},
		{"INFO", LevelInfo},
		{"info", LevelInfo},
		{"WARN", LevelWarn},
		{"warn", LevelWarn},
		{"WARNING", LevelWarn},
		{"warning", LevelWarn},
		{"ERROR", LevelError},
		{"error", LevelError},
		{"", LevelInfo},
		{"invalid", LevelInfo},
		{"TRACE", LevelInfo}, // Unknown levels default to INFO
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := ParseLevel(tc.input)
			if result != tc.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestLevelToSlogLevel(t *testing.T) {
	tests := []struct {
		level    Level
		expected slog.Level
	}{
		{LevelDebug, slog.LevelDebug},
		{LevelInfo, slog.LevelInfo},
		{LevelWarn, slog.LevelWarn},
		{LevelError, slog.LevelError},
	}

	for _, tc := range tests {
		t.Run(tc.level.String(), func(t *testing.T) {
			result := tc.level.ToSlogLevel()
			if result != tc.expected {
				t.Errorf("Level(%v).ToSlogLevel() = %v, want %v", tc.level, result, tc.expected)
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(99), "INFO"}, // Unknown level defaults to INFO
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.level.String()
			if result != tc.expected {
				t.Errorf("Level(%v).String() = %q, want %q", tc.level, result, tc.expected)
			}
		})
	}
}

func TestSetupWithWriter(t *testing.T) {
	var buf bytes.Buffer

	// Setup with INFO level
	logger := SetupWithWriter(LevelInfo, &buf)

	// Debug messages should be filtered out
	logger.Debug("debug message")
	if buf.Len() > 0 {
		t.Error("Debug message should not be logged at INFO level")
	}

	// Info messages should be logged
	buf.Reset()
	logger.Info("info message")
	if buf.Len() == 0 {
		t.Error("Info message should be logged at INFO level")
	}

	// Now test with DEBUG level
	buf.Reset()
	logger = SetupWithWriter(LevelDebug, &buf)

	// Debug messages should now be logged
	logger.Debug("debug message")
	if buf.Len() == 0 {
		t.Error("Debug message should be logged at DEBUG level")
	}
}
