package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"

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

	fileLevel := resolveLogLevel(debugFlag, traceFlag, logLevelFlag, cfg.ResolveLogLevel())
	_, envOK := config.ResolveLogLevelFromEnv()
	stderrLevel := resolveStderrLevel(debugFlag, traceFlag, logLevelFlag, envOK, fileLevel)
	verbose := traceFlag || os.Getenv("GO_APPLY_LOG_VERBOSE") != ""

	logger.SetVerbose(verbose)

	log, cleanup, err := logger.New(logger.Options{
		LogDir:      logDir,
		FileLevel:   fileLevel,
		StderrLevel: stderrLevel,
	})
	if err != nil {
		// New() only returns nil errors per API contract; this is a safeguard.
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
	}
	defer cleanup()

	slog.SetDefault(log)

	if logLevelFlag != "" {
		if _, ok := parseLevelFlag(logLevelFlag); !ok {
			log.Warn("unrecognised --log-level value, using default", "value", logLevelFlag)
		}
	}

	if err := root.Execute(); err != nil {
		level, msg, code := classifyRunError(err)
		switch level {
		case "info":
			log.Info(msg)
		case "warn":
			log.Warn(msg, "error", err)
		default:
			log.Error(msg, "error", err)
		}
		return code
	}
	return 0
}
