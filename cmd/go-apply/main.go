package main

import (
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"

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

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load: %v\n", err)
		// Continue with defaults; CLI will validate later if needed
		cfg = &config.Config{}
	}

	level := cfg.ResolveLogLevel()
	log, cleanup, err := logger.New(logDir, level)
	if err != nil {
		// New() only returns nil errors per API contract; this is a safeguard.
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
	}
	defer cleanup()

	slog.SetDefault(log)

	root := cli.NewRootCommand(version)
	if err := root.Execute(); err != nil {
		log.Error("command failed", "error", err)
		return 1
	}
	return 0
}
