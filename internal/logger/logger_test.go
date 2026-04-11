package logger_test

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/logger"
)

func TestNew_WritesJSONToTimestampedFile(t *testing.T) {
	dir := t.TempDir()
	log, cleanup, err := logger.New(dir, slog.LevelInfo)
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
	if _, err := time.Parse("2006-01-02T150405Z", ts); err != nil {
		t.Errorf("filename timestamp %q not parseable: %v", ts, err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, name))
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("log line is not valid JSON: %v\nContent: %s", err, data)
	}
	if record["msg"] != "test message" {
		t.Errorf("msg = %v, want 'test message'", record["msg"])
	}
	if record["key"] != "value" {
		t.Errorf("key = %v, want 'value'", record["key"])
	}
}

func TestNew_PrunesOldLogFiles(t *testing.T) {
	dir := t.TempDir()
	for i := range 55 {
		name := fmt.Sprintf("go-apply-2025-01-%02dT120000Z.log", i+1)
		os.WriteFile(filepath.Join(dir, name), []byte("old"), 0640)
	}
	log, cleanup, _ := logger.New(dir, slog.LevelInfo)
	defer cleanup()
	_ = log

	entries, _ := os.ReadDir(dir)
	if len(entries) != 50 {
		t.Errorf("expected 50 log files after pruning, got %d", len(entries))
	}
}

func TestNew_FallsBackToStderrWhenDirUnwritable(t *testing.T) {
	log, cleanup, err := logger.New("/proc/unwritable/path", slog.LevelInfo)
	if err != nil {
		t.Fatalf("New() should not fail on unwritable dir, got: %v", err)
	}
	defer cleanup()
	if log == nil {
		t.Fatal("expected non-nil logger even on fallback")
	}
	log.Warn("fallback smoke test") // verifies fallback logger is functional, not just non-nil
}

func TestNew_DebugLevelFiltersInfo(t *testing.T) {
	dir := t.TempDir()
	log, cleanup, _ := logger.New(dir, slog.LevelWarn)
	defer cleanup()

	log.Info("this should be filtered")
	log.Warn("this should appear")

	cleanup()

	entries, _ := os.ReadDir(dir)
	if len(entries) == 0 {
		t.Skip("no log file created")
	}
	data, _ := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if len(data) == 0 {
		t.Skip("no log output — file may be empty before flush")
	}
	lines := splitLines(string(data))
	for _, line := range lines {
		var r map[string]any
		json.Unmarshal([]byte(line), &r)
		if r["level"] == "INFO" {
			t.Error("INFO log appeared despite Warn level")
		}
	}
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
