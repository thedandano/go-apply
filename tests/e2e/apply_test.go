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
	embStub := newEmbedderStub(t)
	defer embStub.Close()

	env := seedXDGEnv(t, orchStub.URL, embStub.URL)

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
	embStub := newEmbedderStub(t)
	defer embStub.Close()

	env := seedXDGEnv(t, orchStub.URL, embStub.URL)

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
	const wantBestScore = 71.5 // deterministic: 5/7 required keywords + experience years
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

// TestRun_VectorCacheBehavior asserts keyword-vector cache hit/miss semantics:
//   - First run: each JD keyword produces a "keyword vector cache miss" log record.
//   - Second run (same JD, same XDG env): each keyword produces a "keyword vector cache hit"
//     record and zero cache-miss records.
//
// Invariant: the SQLite keyword-vector cache persists across process invocations.
func TestRun_VectorCacheBehavior(t *testing.T) {
	binary := buildBinary(t)

	orchStub := newOrchestratorStub(t)
	defer orchStub.Close()
	embStub := newEmbedderStub(t)
	defer embStub.Close()

	env := seedXDGEnv(t, orchStub.URL, embStub.URL)

	// Onboard so the pipeline can proceed past the guard.
	onboardCmd := exec.Command(binary, "onboard",
		"--resume", filepath.Join("testdata", "resume_backend.txt"),
		"--skills", filepath.Join("testdata", "skills.md"),
		"--accomplishments", filepath.Join("testdata", "accomplishments.md"),
	)
	onboardCmd.Env = env.Environ
	if out, err := onboardCmd.CombinedOutput(); err != nil {
		t.Fatalf("onboard failed: %v\noutput: %s", err, out)
	}

	// Augmentation only runs when --accomplishments is supplied.
	// Both runs share the same XDG env so the vector cache persists between them.
	jdText := "Senior Backend Engineer at Acme Corp. Required: Go, Kubernetes, PostgreSQL, gRPC, Docker."
	accomplishmentsPath := filepath.Join("testdata", "accomplishments.md")

	doRun := func() {
		t.Helper()
		cmd := exec.Command(binary, "run", "--text", jdText, "--accomplishments", accomplishmentsPath)
		cmd.Env = env.Environ
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("run failed: %v\noutput: %s", err, out)
		}
	}

	// Record log count after onboard so we can slice per-run records later.
	onboardCount := len(readLogFile(t, env.StateDir))

	// First run: cold cache — expect misses, no hits.
	doRun()
	allAfterRun1 := readLogFile(t, env.StateDir)
	run1Logs := allAfterRun1[onboardCount:]
	if !hasLogRecord(run1Logs, "msg", "keyword vector cache miss") {
		t.Error("first run: expected at least one 'keyword vector cache miss' record")
	}
	if hasLogRecord(run1Logs, "msg", "keyword vector cache hit") {
		t.Error("first run: unexpected 'keyword vector cache hit' — cache should be cold")
	}

	// Second run: warm cache — expect hits, no misses.
	run1Count := len(allAfterRun1)
	doRun()
	allAfterRun2 := readLogFile(t, env.StateDir)
	run2Logs := allAfterRun2[run1Count:]
	if !hasLogRecord(run2Logs, "msg", "keyword vector cache hit") {
		t.Error("second run: expected at least one 'keyword vector cache hit' record")
	}
	if hasLogRecord(run2Logs, "msg", "keyword vector cache miss") {
		t.Error("second run: unexpected 'keyword vector cache miss' — vectors should be cached")
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
