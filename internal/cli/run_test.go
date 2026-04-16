package cli_test

import (
	"strings"
	"testing"
)

// executeRun runs the root command with "run" subcommand and given args,
// reusing the executeSetupMCP helper pattern.
func executeRun(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return executeSetupMCP(t, append([]string{"run"}, args...)...)
}

// TestCLIRun_NoOrchestrator_ExitsNonZero verifies that when the config has no
// orchestrator base_url or model set, the run command returns a non-zero exit
// with the expected guidance message.
func TestCLIRun_NoOrchestrator_ExitsNonZero(t *testing.T) {
	// Override XDG_CONFIG_HOME to an empty temp dir so config.Load()
	// creates a fresh empty config (no orchestrator configured).
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("GO_APPLY_API_KEY", "") // ensure no env override

	_, _, err := executeRun(t, "--text", "some job description")
	if err == nil {
		t.Fatal("expected non-zero exit when orchestrator is not configured, got nil")
	}

	combined := err.Error()
	if !strings.Contains(combined, "no orchestrator configured") {
		t.Errorf("expected error to contain 'no orchestrator configured', got: %v", err)
	}
}

// TestCLIRun_MutuallyExclusiveFlags verifies that --url and --text cannot both be set.
func TestCLIRun_MutuallyExclusiveFlags(t *testing.T) {
	_, _, err := executeRun(t, "--url", "https://example.com/job", "--text", "raw jd text")
	if err == nil {
		t.Fatal("expected error when --url and --text are both set, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error, got: %v", err)
	}
}

// TestCLIRun_MissingInputFlag verifies that at least one of --url or --text is required.
func TestCLIRun_MissingInputFlag(t *testing.T) {
	_, _, err := executeRun(t)
	if err == nil {
		t.Fatal("expected error when neither --url nor --text is set, got nil")
	}
}
