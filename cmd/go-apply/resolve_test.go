package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
)

func TestResolveLogLevelAndStderrLevel(t *testing.T) {
	// Note: subtests use t.Setenv, so this test must NOT use t.Parallel()
	// at the subtest level. The top-level is sequential.

	tests := []struct {
		name          string
		debug         bool
		trace         bool
		flagVal       string
		envVal        string // set GO_APPLY_LOG_LEVEL to this (empty = unset)
		cfgLevel      slog.Level
		wantFileLevel slog.Level
		wantStderr    slog.Level
	}{
		{
			name:          "--debug flag → fileLevel=DEBUG, stderrLevel=DEBUG",
			debug:         true,
			cfgLevel:      slog.LevelInfo,
			wantFileLevel: slog.LevelDebug,
			wantStderr:    slog.LevelDebug,
		},
		{
			name:          "--trace flag → fileLevel=DEBUG, stderrLevel=DEBUG",
			trace:         true,
			cfgLevel:      slog.LevelInfo,
			wantFileLevel: slog.LevelDebug,
			wantStderr:    slog.LevelDebug,
		},
		{
			name:          "--log-level=error → fileLevel=ERROR, stderrLevel=ERROR (regression: no inversion)",
			flagVal:       "error",
			cfgLevel:      slog.LevelInfo,
			wantFileLevel: slog.LevelError,
			wantStderr:    slog.LevelError,
		},
		{
			name:          "--log-level=warn → fileLevel=WARN, stderrLevel=WARN",
			flagVal:       "warn",
			cfgLevel:      slog.LevelInfo,
			wantFileLevel: slog.LevelWarn,
			wantStderr:    slog.LevelWarn,
		},
		{
			name:          "env GO_APPLY_LOG_LEVEL=debug → fileLevel=DEBUG, stderrLevel=DEBUG",
			envVal:        "debug",
			cfgLevel:      slog.LevelInfo,
			wantFileLevel: slog.LevelDebug,
			wantStderr:    slog.LevelDebug,
		},
		{
			name:          "no flags, no env, cfg=INFO → fileLevel=INFO, stderrLevel=WARN",
			cfgLevel:      slog.LevelInfo,
			wantFileLevel: slog.LevelInfo,
			wantStderr:    slog.LevelWarn,
		},
		{
			name:          "invalid --log-level=xyz, no env, cfg=INFO → fileLevel=INFO (falls through to config)",
			flagVal:       "xyz",
			cfgLevel:      slog.LevelInfo,
			wantFileLevel: slog.LevelInfo,
			wantStderr:    slog.LevelWarn,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// t.Setenv requires sequential subtests (not parallel).
			// Explicitly set to empty string to clear any inherited env value.
			if tc.envVal != "" {
				t.Setenv("GO_APPLY_LOG_LEVEL", tc.envVal)
			} else {
				t.Setenv("GO_APPLY_LOG_LEVEL", "")
			}

			fileLevel := resolveLogLevel(tc.debug, tc.trace, tc.flagVal, tc.cfgLevel)
			_, envOK := config.ResolveLogLevelFromEnv()
			stderrLevel := resolveStderrLevel(tc.debug, tc.trace, tc.flagVal, envOK, fileLevel)

			if fileLevel != tc.wantFileLevel {
				t.Errorf("fileLevel = %s, want %s", fileLevel, tc.wantFileLevel)
			}
			if stderrLevel != tc.wantStderr {
				t.Errorf("stderrLevel = %s, want %s", stderrLevel, tc.wantStderr)
			}
		})
	}
}

func TestParseLevelFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		wantLvl slog.Level
		wantOK  bool
	}{
		{"debug", slog.LevelDebug, true},
		{"DEBUG", slog.LevelDebug, true},
		{"info", slog.LevelInfo, true},
		{"warn", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"xyz", slog.LevelInfo, false},
		{"", slog.LevelInfo, false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, ok := parseLevelFlag(tc.input)
			if ok != tc.wantOK {
				t.Errorf("parseLevelFlag(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
			}
			if got != tc.wantLvl {
				t.Errorf("parseLevelFlag(%q) level = %s, want %s", tc.input, got, tc.wantLvl)
			}
		})
	}
}

func TestClassifyRunError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantLvl  string
		wantMsg  string
		wantCode int
	}{
		{
			name:     "context.Canceled",
			err:      context.Canceled,
			wantLvl:  "info",
			wantMsg:  "command canceled",
			wantCode: 130,
		},
		{
			name:     "wrapped context.Canceled",
			err:      fmt.Errorf("doing X: %w", context.Canceled),
			wantLvl:  "info",
			wantMsg:  "command canceled",
			wantCode: 130,
		},
		{
			name:     "context.DeadlineExceeded",
			err:      context.DeadlineExceeded,
			wantLvl:  "warn",
			wantMsg:  "command timed out",
			wantCode: 1,
		},
		{
			name:     "wrapped context.DeadlineExceeded",
			err:      fmt.Errorf("waiting: %w", context.DeadlineExceeded),
			wantLvl:  "warn",
			wantMsg:  "command timed out",
			wantCode: 1,
		},
		{
			name:     "generic error",
			err:      errors.New("boom"),
			wantLvl:  "error",
			wantMsg:  "command failed",
			wantCode: 1,
		},
		{
			name:     "wrapped generic error",
			err:      fmt.Errorf("failed to execute: %w", errors.New("boom")),
			wantLvl:  "error",
			wantMsg:  "command failed",
			wantCode: 1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotLvl, gotMsg, gotCode := classifyRunError(tc.err)
			if gotLvl != tc.wantLvl {
				t.Errorf("level = %q, want %q", gotLvl, tc.wantLvl)
			}
			if gotMsg != tc.wantMsg {
				t.Errorf("msg = %q, want %q", gotMsg, tc.wantMsg)
			}
			if gotCode != tc.wantCode {
				t.Errorf("code = %d, want %d", gotCode, tc.wantCode)
			}
		})
	}
}
