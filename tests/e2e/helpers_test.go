//go:build e2e

package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
)

// testEnv holds XDG-sandboxed directory paths and the subprocess env slice
// constructed by seedXDGEnv. All three XDG dirs are distinct temp directories
// owned by the test and cleaned up automatically via t.TempDir().
type testEnv struct {
	ConfigDir string // $XDG_CONFIG_HOME — config.yaml written here
	DataDir   string // $XDG_DATA_HOME   — profile.db created here
	StateDir  string // $XDG_STATE_HOME  — log files written here
	Environ   []string
}

// seedXDGEnv creates an isolated XDG environment for a subprocess test.
// Sets LOG_FORMAT=json and LOG_LEVEL=debug so log assertions work, and returns
// the constructed testEnv.
func seedXDGEnv(t *testing.T) *testEnv {
	t.Helper()

	configDir := t.TempDir()
	dataDir := t.TempDir()
	stateDir := t.TempDir()

	goApplyCfgDir := filepath.Join(configDir, "go-apply")
	if err := os.MkdirAll(goApplyCfgDir, config.DirPerm); err != nil {
		t.Fatalf("create config dir: %v", err)
	}

	// Pre-create data subdirectories so the resume repository (go-apply/inputs/)
	// doesn't fail on missing parent directories.
	for _, sub := range []string{"go-apply", filepath.Join("go-apply", "inputs")} {
		if err := os.MkdirAll(filepath.Join(dataDir, sub), 0o700); err != nil {
			t.Fatalf("create data subdir %s: %v", sub, err)
		}
	}

	if err := os.WriteFile(filepath.Join(goApplyCfgDir, "config.yaml"), []byte(""), config.FilePerm); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	env := append(os.Environ(),
		"XDG_CONFIG_HOME="+configDir,
		"XDG_DATA_HOME="+dataDir,
		"XDG_STATE_HOME="+stateDir,
		"LOG_FORMAT=json",
		"LOG_LEVEL=debug",
	)

	return &testEnv{
		ConfigDir: configDir,
		DataDir:   dataDir,
		StateDir:  stateDir,
		Environ:   env,
	}
}

// readLogFile finds all *.log files under $stateDir/go-apply/logs/, parses
// each line as a JSON object, and returns the combined slice of log records.
// Fails the test if the log directory does not exist or contains no files.
func readLogFile(t *testing.T, stateDir string) []map[string]any {
	t.Helper()

	logDir := filepath.Join(stateDir, "go-apply", "logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("read log dir %s: %v", logDir, err)
	}

	var records []map[string]any
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(logDir, e.Name()))
		if err != nil {
			t.Fatalf("read log file %s: %v", e.Name(), err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue // skip malformed lines
			}
			records = append(records, rec)
		}
	}

	if len(records) == 0 {
		t.Fatalf("no log records found in %s", logDir)
	}
	return records
}

// readFixture reads a test fixture file and returns its trimmed contents.
func readFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return strings.TrimSpace(string(data))
}

// requireJSONEq fails the test if expected and actual are not deeply equal.
func requireJSONEq(t *testing.T, expected, actual map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		e, _ := json.MarshalIndent(expected, "", "  ")
		a, _ := json.MarshalIndent(actual, "", "  ")
		t.Errorf("JSON mismatch:\nwant: %s\ngot:  %s", e, a)
	}
}

// hasLogRecord reports whether any log record has the given key set to value.
func hasLogRecord(logs []map[string]any, key, value string) bool {
	for _, r := range logs {
		if v, ok := r[key].(string); ok && v == value {
			return true
		}
	}
	return false
}

// requireNoErrors fails the test for every log record whose level is "ERROR".
func requireNoErrors(t *testing.T, logs []map[string]any) {
	t.Helper()
	for _, r := range logs {
		level, _ := r["level"].(string)
		if strings.EqualFold(level, "ERROR") {
			msg, _ := r["msg"].(string)
			t.Errorf("unexpected ERROR log record: %s (full record: %v)", msg, r)
		}
	}
}
