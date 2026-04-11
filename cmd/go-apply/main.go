package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/thedandano/go-apply/internal/cli"
	"github.com/thedandano/go-apply/internal/logger"
)

func main() {
	// TODO(task-2): replace with config.LogDir() once config package is implemented
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".local", "state", "go-apply", "logs")

	log, cleanup, err := logger.New(logDir, slog.LevelInfo)
	if err != nil {
		// New() only returns nil errors per API contract; this is a safeguard.
		fmt.Fprintf(os.Stderr, "logger init: %v\n", err)
	}
	defer cleanup()

	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		log.Error("command failed", "error", err)
		os.Exit(1)
	}
}
