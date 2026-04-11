//go:build e2e

package e2e_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestApplyHeadless_GoldenPath is the permanent integration smoke test.
// Written in Task 0 — stays RED until Epic 3 headless pipeline is complete.
func TestApplyHeadless_GoldenPath(t *testing.T) {
	binary := buildBinary(t)

	jdPath := filepath.Join("testdata", "jd_sample.txt")
	jdBytes, err := os.ReadFile(jdPath)
	if err != nil {
		t.Fatalf("read jd fixture: %v", err)
	}

	cmd := exec.Command(binary, "apply", "--headless", "--text", string(jdBytes))
	cmd.Env = append(os.Environ(), "GO_APPLY_API_KEY=test-key")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go-apply apply failed: %v\nstderr: %s", err, cmd.Stderr)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, out)
	}
	if result["status"] != "success" {
		t.Errorf("status = %v, want success", result["status"])
	}
	if result["best_score"] == nil || result["best_score"].(float64) == 0 {
		t.Error("best_score is 0 or missing — scoring did not run")
	}
}

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
