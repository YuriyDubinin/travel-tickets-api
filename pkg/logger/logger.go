// Package logger constructs the application's structured slog logger.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New returns a *slog.Logger configured for the given level and environment.
// In production it emits JSON; otherwise it emits human-readable text. An
// unrecognized level falls back to info. Output is written to stdout.
func New(level, env string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}

	var handler slog.Handler
	if env == "production" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// parseLevel maps a textual level to slog.Level, defaulting to info.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
