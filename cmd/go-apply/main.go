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
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load: %v\n", err)
		cfg = &config.Config{}
	}

	level := cfg.ResolveLogLevel()
	logger.SetVerbose(cfg.Verbose)

	log, cleanup, logErr := logger.New(logger.Options{
		LogDir:      config.LogDir(),
		FileLevel:   level,
		StderrLevel: level,
	})
	if logErr != nil {
		fmt.Fprintf(os.Stderr, "logger init: %v\n", logErr)
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
