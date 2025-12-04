package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Level represents log verbosity levels
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// ParseLevel converts a string level name to Level
func ParseLevel(s string) Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	default:
		return LevelInfo
	}
}

// ToSlogLevel converts our Level to slog.Level
func (l Level) ToSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// String returns the string representation of the level
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// Setup initializes the global slog logger with the specified level
func Setup(level Level) *slog.Logger {
	return SetupWithWriter(level, os.Stderr)
}

// SetupWithWriter initializes slog with a custom writer (useful for testing)
func SetupWithWriter(level Level, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: level.ToSlogLevel(),
	}
	handler := slog.NewTextHandler(w, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
