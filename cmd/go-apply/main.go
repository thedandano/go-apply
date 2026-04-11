package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/thedandano/go-apply/internal/cli"
	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
)

func main() {
	os.Exit(run())
}

func run() int {
	logDir := config.LogDir()

	log, cleanup, err := logger.New(logDir, slog.LevelInfo)
	if err != nil {
		// New() only returns nil errors per API contract; this is a safeguard.
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
	}
	defer cleanup()

	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		log.Error("command failed", "error", err)
		return 1
	}
	return 0
}
