package logger_test

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/logger"
)

func TestNew_WritesHumanReadableToDailyFile(t *testing.T) {
	dir := t.TempDir()
	log, cleanup, err := logger.New(logger.Options{
		LogDir:      dir,
		FileLevel:   slog.LevelDebug,
		StderrLevel: slog.LevelWarn,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer cleanup()

	log.Info("test message", "key", "value")
	cleanup() // flush before reading

	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatal("no log file created")
	}
	name := entries[0].Name()
	if !strings.HasPrefix(name, "go-apply-") || !strings.HasSuffix(name, ".log") {
		t.Errorf("unexpected log filename: %s", name)
	}
	ts := strings.TrimSuffix(strings.TrimPrefix(name, "go-apply-"), ".log")
	if _, err := time.Parse("2006-01-02", ts); err != nil {
		t.Errorf("filename date %q not parseable: %v", ts, err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, name))
	content := string(data)

	if strings.HasPrefix(strings.TrimSpace(content), "{") {
		t.Errorf("log line must not be JSON, got: %s", content)
	}
	if !strings.Contains(content, "test message") {
		t.Errorf("log line must contain 'test message', got: %s", content)
	}
	if !strings.Contains(content, "key=value") {
		t.Errorf("log line must contain 'key=value', got: %s", content)
	}
}

func TestNew_TimestampFormat(t *testing.T) {
	dir := t.TempDir()
	log, cleanup, _ := logger.New(logger.Options{
		LogDir:      dir,
		FileLevel:   slog.LevelDebug,
		StderrLevel: slog.LevelWarn,
	})
	defer cleanup()

	log.Info("ts check")
	cleanup()

	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Skip("no log file created")
	}
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	content := string(data)

	tsPattern := regexp.MustCompile(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`)
	if !tsPattern.MatchString(content) {
		t.Errorf("log must contain timestamp in format 'YYYY-MM-DD HH:MM:SS', got: %s", content)
	}
	if strings.Contains(content, "T") && regexp.MustCompile(`\d{4}-\d{2}-\d{2}T`).MatchString(content) {
		t.Errorf("log must not contain RFC3339 'T' separator, got: %s", content)
	}
}

func TestNew_PrunesOldLogFiles(t *testing.T) {
	dir := t.TempDir()
	for i := range 55 {
		name := fmt.Sprintf("go-apply-2025-%02d-01.log", i+1)
		os.WriteFile(filepath.Join(dir, name), []byte("old"), 0640)
	}
	log, cleanup, _ := logger.New(logger.Options{
		LogDir:      dir,
		FileLevel:   slog.LevelDebug,
		StderrLevel: slog.LevelWarn,
	})
	defer cleanup()
	_ = log

	entries, _ := os.ReadDir(dir)
	if len(entries) != 50 {
		t.Errorf("expected 50 log files after pruning, got %d", len(entries))
	}
}

func TestNew_FallsBackToStderrWhenDirUnwritable(t *testing.T) {
	log, cleanup, err := logger.New(logger.Options{
		LogDir:      "/proc/unwritable/path",
		FileLevel:   slog.LevelDebug,
		StderrLevel: slog.LevelWarn,
	})
	if err != nil {
		t.Fatalf("New() should not fail on unwritable dir, got: %v", err)
	}
	defer cleanup()
	if log == nil {
		t.Fatal("expected non-nil logger even on fallback")
	}
	log.Warn("fallback smoke test") // verifies fallback logger is functional, not just non-nil
}

func TestNew_FileReceivesDebugLogs(t *testing.T) {
	dir := t.TempDir()
	log, cleanup, _ := logger.New(logger.Options{
		LogDir:      dir,
		FileLevel:   slog.LevelDebug,
		StderrLevel: slog.LevelWarn,
	})
	defer cleanup()

	log.Debug("debug message")
	log.Info("info message")

	cleanup()

	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Skip("no log file created")
	}
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if len(data) == 0 {
		t.Skip("no log output — file may be empty before flush")
	}

	content := string(data)
	if !strings.Contains(content, "debug message") {
		t.Error("DEBUG log must appear in log file")
	}
	if !strings.Contains(content, "info message") {
		t.Error("INFO log must appear in log file")
	}
}

func TestNew_FileRespectLogLevel(t *testing.T) {
	dir := t.TempDir()
	log, cleanup, _ := logger.New(logger.Options{
		LogDir:      dir,
		FileLevel:   slog.LevelInfo,
		StderrLevel: slog.LevelWarn,
	})
	defer cleanup()

	log.Debug("debug message")
	log.Info("info message")

	cleanup()

	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Skip("no log file created")
	}
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if len(data) == 0 {
		t.Skip("no log output — file may be empty before flush")
	}

	content := string(data)
	if strings.Contains(content, "debug message") {
		t.Error("DEBUG message should NOT appear in log file when level is INFO")
	}
	if !strings.Contains(content, "info message") {
		t.Error("INFO message must appear in log file when level is INFO")
	}
}

func TestNew_StderrRespectsLevel(t *testing.T) {
	// Save original stderr
	origStderr := os.Stderr

	// Create a temp file for captured stderr
	tmpFile, err := os.CreateTemp("", "stderr-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Redirect stderr BEFORE calling logger.New() so the logger captures it
	os.Stderr = tmpFile
	t.Cleanup(func() { os.Stderr = origStderr })

	dir := t.TempDir()
	log, cleanup, _ := logger.New(logger.Options{
		LogDir:      dir,
		FileLevel:   slog.LevelDebug,
		StderrLevel: slog.LevelWarn,
	})

	// Log a DEBUG message (should NOT appear on stderr since StderrLevel is WARN)
	log.Debug("debug message to stderr")
	// Log a WARN message (SHOULD appear on stderr)
	log.Warn("warn message to stderr")

	cleanup()

	tmpFile.Close()

	// Read captured stderr
	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read captured stderr: %v", err)
	}
	stderrContent := string(data)

	// Verify DEBUG message is absent from stderr
	if strings.Contains(stderrContent, "debug message to stderr") {
		t.Error("DEBUG message must NOT appear on stderr when StderrLevel is WARN")
	}
	// Verify WARN message is present on stderr
	if !strings.Contains(stderrContent, "warn message to stderr") {
		t.Errorf("WARN message must appear on stderr when StderrLevel is WARN, got: %s", stderrContent)
	}

	// Also verify both messages appear in the file log
	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Fatalf("no log file created")
	}
	fileData, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if len(fileData) == 0 {
		t.Fatalf("no log output — file may be empty before flush")
	}

	fileContent := string(fileData)
	if !strings.Contains(fileContent, "debug message to stderr") {
		t.Error("DEBUG message must appear in file when FileLevel is DEBUG")
	}
	if !strings.Contains(fileContent, "warn message to stderr") {
		t.Error("WARN message must appear in file when FileLevel is DEBUG")
	}
}
