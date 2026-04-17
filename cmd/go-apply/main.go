package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/cli"
	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
)

var version = "dev"

func init() {
	// When installed via `go install`, ldflags are not applied.
	// Fall back to the module version embedded by the Go toolchain.
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
}

func main() {
	os.Exit(run())
}

func run() int {
	logDir := config.LogDir()

	root := cli.NewRootCommand(version)

	// Pre-parse to extract log flags before logger init.
	// UnknownFlags:true lets us ignore subcommand names and flags that are
	// not registered on the root command.
	root.FParseErrWhitelist = cobra.FParseErrWhitelist{UnknownFlags: true}
	_ = root.ParseFlags(os.Args[1:])
	root.FParseErrWhitelist = cobra.FParseErrWhitelist{} // restore: unknown flags should error during Execute

	debugFlag, _ := root.PersistentFlags().GetBool("debug")
	traceFlag, _ := root.PersistentFlags().GetBool("trace")
	logLevelFlag, _ := root.PersistentFlags().GetString("log-level")

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load: %v\n", err)
		// Continue with defaults; CLI will validate later if needed.
		cfg = &config.Config{}
	}

	level := resolveLogLevel(debugFlag, traceFlag, logLevelFlag, cfg)
	stderrLevel := resolveStderrLevel(debugFlag, traceFlag, logLevelFlag)
	verbose := traceFlag || os.Getenv("GO_APPLY_LOG_VERBOSE") != ""

	logger.SetVerbose(verbose)

	log, cleanup, err := logger.New(logger.Options{
		LogDir:      logDir,
		FileLevel:   level,
		StderrLevel: stderrLevel,
	})
	if err != nil {
		// New() only returns nil errors per API contract; this is a safeguard.
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
	}
	defer cleanup()

	slog.SetDefault(log)

	if err := root.Execute(); err != nil {
		log.Error("command failed", "error", err)
		return 1
	}
	return 0
}

// resolveLogLevel applies precedence: flag > env > config > default (INFO).
func resolveLogLevel(debug, trace bool, flagVal string, cfg *config.Config) slog.Level {
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
	return cfg.ResolveLogLevel()
}

// resolveStderrLevel keeps stderr at WARN unless debug is explicitly requested.
// This preserves a clean TUI experience for non-debug invocations.
// Only debug-level requests (via flag or env) promote stderr output.
func resolveStderrLevel(debug, trace bool, flagVal string) slog.Level {
	if trace || debug {
		return slog.LevelDebug
	}
	if strings.EqualFold(flagVal, "debug") {
		return slog.LevelDebug
	}
	if l, ok := config.ResolveLogLevelFromEnv(); ok && l == slog.LevelDebug {
		return l
	}
	return slog.LevelWarn
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
