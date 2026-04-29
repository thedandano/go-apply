package mcpserver_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain redirects XDG_DATA_HOME to a temp directory so that handler tests
// do not write to the real ~/.local/share/go-apply/ directory.
// Required subdirectories (inputs/, sessions/) are pre-created so handlers
// that expect them (add_resume, session persistence) do not fail on mkdir.
func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "go-apply-test-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	if err := os.Setenv("XDG_DATA_HOME", tmpDir); err != nil {
		panic(err)
	}
	base := filepath.Join(tmpDir, "go-apply")
	for _, sub := range []string{"inputs", "sessions"} {
		if err := os.MkdirAll(filepath.Join(base, sub), 0o755); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}
