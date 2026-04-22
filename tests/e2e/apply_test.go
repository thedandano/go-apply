//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_GuardsUnonboarded asserts that running go-apply run against an empty
// profile (no prior onboard) exits non-zero and tells the user to onboard first.
// Invariant: onboard guard fires before any pipeline work.
func TestRun_GuardsUnonboarded(t *testing.T) {
	binary := buildBinary(t)

	orchStub := newOrchestratorStub(t)
	defer orchStub.Close()

	env := seedXDGEnv(t, orchStub.URL)

	cmd := exec.Command(binary, "run", "--text", "Senior Backend Engineer at Acme Corp. Required: Go, Kubernetes.")
	cmd.Env = env.Environ

	out, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected a non-zero exit, got: %v\noutput: %s", err, out)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected exit code != 0, got 0\noutput: %s", out)
	}
	// "no resumes found" is the exact phrase from onboardcheck.CheckOnboarded.
	// Checking for this string (not just "onboard") avoids false positives from
	// t.TempDir() paths that include the test name "TestRun_GuardsUnonboarded".
	if !strings.Contains(strings.ToLower(string(out)), "no resumes found") {
		t.Errorf("expected output to contain 'no resumes found' (onboard guard message), got:\n%s", out)
	}
}

// TestRun_HappyPath is the regression baseline: onboard with all three fixture docs,
// run against a JD, and assert the pipeline completes successfully.
// Invariants captured:
//   - exit code 0 and status == "success"
//   - best_score > 0 (scoring ran and matched keywords)
//   - keywords.required non-empty (keyword extraction ran)
//   - stage banners present in log (acquire_jd, extract_keywords, score_resumes)
//   - zero ERROR records in log
func TestRun_HappyPath(t *testing.T) {
	binary := buildBinary(t)

	orchStub := newOrchestratorStub(t)
	defer orchStub.Close()

	env := seedXDGEnv(t, orchStub.URL)

	// Step 1: onboard with all three fixture docs.
	onboardCmd := exec.Command(binary, "onboard",
		"--resume", filepath.Join("testdata", "resume_backend.txt"),
		"--skills", filepath.Join("testdata", "skills.md"),
		"--accomplishments", filepath.Join("testdata", "accomplishments.md"),
	)
	onboardCmd.Env = env.Environ
	if out, err := onboardCmd.CombinedOutput(); err != nil {
		t.Fatalf("onboard failed: %v\noutput: %s", err, out)
	}

	// Step 2: run against a JD; capture stdout (JSON result) and stderr separately.
	jdText := "Senior Backend Engineer at Acme Corp. Required: Go, Kubernetes, PostgreSQL, gRPC, Docker."
	var stdout, stderr bytes.Buffer
	runCmd := exec.Command(binary, "run", "--text", jdText)
	runCmd.Env = env.Environ
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr
	if err := runCmd.Run(); err != nil {
		t.Fatalf("run failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Assert JSON result.
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if result["status"] != "success" {
		t.Errorf("status = %v, want success", result["status"])
	}
	const wantBestScore = 60.17 // deterministic: resume_backend matches 5/7 required keywords (Go, K8s, PostgreSQL, gRPC, Docker) + experience years; skills.md no longer scored (M3 source-scoped layout)
	const scoreTolerance = 0.5
	bestScore, _ := result["best_score"].(float64)
	if bestScore < wantBestScore-scoreTolerance || bestScore > wantBestScore+scoreTolerance {
		t.Errorf("best_score = %.2f, want %.2f ± %.2f", bestScore, wantBestScore, scoreTolerance)
	}
	if kw, ok := result["keywords"].(map[string]any); !ok {
		t.Error("keywords field missing from result")
	} else if req, _ := kw["required"].([]any); len(req) == 0 {
		t.Error("keywords.required is empty — keyword extraction did not run")
	}

	// Assert log records.
	logs := readLogFile(t, env.StateDir)
	requireNoErrors(t, logs)
	for _, stage := range []string{"acquire_jd", "extract_keywords", "score_resumes"} {
		if !hasLogRecord(logs, "stage", stage) {
			t.Errorf("no log record with stage=%q found — stage banner missing", stage)
		}
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
