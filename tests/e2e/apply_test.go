//go:build e2e

package e2e_test

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMCPServer_ApplyTool verifies the MCP server responds to apply_to_job.
// Written in Task 0 — stays RED until Epic 4.
func TestMCPServer_ApplyTool(t *testing.T) {
	binary := buildBinary(t)
	_ = binary
	t.Skip("MCP e2e test scaffolded — implemented in Epic 4")
}

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "go-apply")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/go-apply/")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", out, err)
	}
	return bin
}
