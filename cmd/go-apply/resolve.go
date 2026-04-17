package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
)

// resolveLogLevel applies precedence: flag > env > config > default (INFO).
// cfgLevel is the already-resolved level from config (cfg.ResolveLogLevel()).
func resolveLogLevel(debug, trace bool, flagVal string, cfgLevel slog.Level) slog.Level {
	if trace || debug {
		return slog.LevelDebug
	}
	if flagVal != "" {
		if l, ok := parseLevelFlag(flagVal); ok {
			return l
		}
	}
	if l, ok := config.ResolveLogLevelFromEnv(); ok {
		return l
	}
	return cfgLevel
}

// resolveStderrLevel keeps stderr at WARN unless a log level is explicitly requested.
// When the user provides a valid explicit level (via flag, env, or debug/trace shortcuts),
// stderr mirrors the file level to preserve consistency.
// Invalid flag values are treated as no explicit request — stderr stays at WARN.
// This preserves a clean TUI experience for non-debug invocations.
func resolveStderrLevel(debug, trace bool, flagVal string, envRequested bool, fileLevel slog.Level) slog.Level {
	validFlag := flagVal != "" && func() bool { _, ok := parseLevelFlag(flagVal); return ok }()
	if trace || debug || validFlag || envRequested {
		return fileLevel // explicit request: stderr mirrors file level
	}
	return slog.LevelWarn // no explicit request: keep TUI clean
}

// parseLevelFlag converts a flag string to an slog.Level.
// Returns (LevelInfo, false) for unrecognised values.
func parseLevelFlag(s string) (slog.Level, bool) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return slog.LevelInfo, false
}

// classifyRunError returns the log level, message, and exit code for a cobra.Command.Execute() error.
// It classifies context.Canceled as INFO with exit code 130,
// context.DeadlineExceeded as WARN with exit code 1,
// and all other errors as ERROR with exit code 1.
func classifyRunError(err error) (level string, msg string, code int) {
	switch {
	case errors.Is(err, context.Canceled):
		return "info", "command canceled", 130
	case errors.Is(err, context.DeadlineExceeded):
		return "warn", "command timed out", 1
	default:
		return "error", "command failed", 1
	}
}
