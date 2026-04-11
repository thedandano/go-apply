package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxLogFiles   = 50
	logFilePrefix = "go-apply-"
)

// New creates a *slog.Logger writing JSON lines to a timestamped log file in logDir.
//
// File naming: go-apply-2006-01-02T150405Z.log (one per invocation)
// Dual output: file at configured level + stderr at WARN+ (keeps TUI clean)
// Retention: keeps the last maxLogFiles files, prunes older ones on startup
// Fallback: if logDir is unwritable → stderr-only logger, no error returned
func New(logDir string, level slog.Level) (*slog.Logger, func(), error) {
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return stderrOnly(level), func() {}, nil
	}

	// Keep maxLogFiles-1 existing files so the new file below brings the total to maxLogFiles.
	pruneOldLogs(logDir, maxLogFiles-1)

	timestamp := time.Now().UTC().Format("2006-01-02T150405Z")
	logPath := filepath.Join(logDir, logFilePrefix+timestamp+".log")

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600) // #nosec G304 -- logPath built from os.UserHomeDir() + fixed suffix, not user input
	if err != nil {
		return stderrOnly(level), func() {}, nil
	}

	log := slog.New(&multiHandler{
		file:   slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level}),
		stderr: slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}),
	})

	var once sync.Once
	return log, func() { once.Do(func() { _ = f.Close() }) }, nil //nolint:gosec // G104: close error in cleanup is non-fatal
}

func stderrOnly(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func pruneOldLogs(logDir string, keep int) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	var logFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), logFilePrefix) && strings.HasSuffix(e.Name(), ".log") {
			logFiles = append(logFiles, filepath.Join(logDir, e.Name()))
		}
	}
	sort.Strings(logFiles)
	if len(logFiles) <= keep {
		return
	}
	for _, old := range logFiles[:len(logFiles)-keep] {
		_ = os.Remove(old) //nolint:gosec // G104: best-effort log rotation deletion
	}
}

type multiHandler struct {
	file   slog.Handler
	stderr slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.file.Enabled(ctx, level) || h.stderr.Enabled(ctx, level)
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires slog.Record by value
	if h.file.Enabled(ctx, r.Level) {
		if err := h.file.Handle(ctx, r); err != nil {
			return fmt.Errorf("file handler: %w", err)
		}
	}
	if h.stderr.Enabled(ctx, r.Level) {
		_ = h.stderr.Handle(ctx, r.Clone()) //nolint:gosec // G104: stderr write failure is non-fatal
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{
		file:   h.file.WithAttrs(attrs),
		stderr: h.stderr.WithAttrs(attrs),
	}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{
		file:   h.file.WithGroup(name),
		stderr: h.stderr.WithGroup(name),
	}
}
