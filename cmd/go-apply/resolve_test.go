package main

import (
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
