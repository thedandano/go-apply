package main

import (
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

	log, cleanup, _ := logger.New(logDir, slog.LevelInfo)
	defer cleanup()

	root := cli.NewRootCommand()
	if err := root.Execute(); err != nil {
		log.Error("command failed", "error", err)
		os.Exit(1)
	}
}
