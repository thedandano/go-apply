package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
)

// TestRunCommandRemoved is RED until T008/T009 delete the "run" command.
// It asserts that `go-apply run` returns a non-zero exit code with
// "unknown command" in the output (cobra's standard message).
//
// Today, this test FAILS because the "run" command exists and executes successfully.
// After the run command is removed, this test must PASS.
func TestRunCommandRemoved(t *testing.T) {
	// Build the binary into a temp directory.
	tmpDir := t.TempDir()
	binaryPath := filepath.Join(tmpDir, "go-apply")

	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	buildCmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/go-apply") //nolint:noctx
	buildCmd.Dir = projectRoot
	buildOutput := new(bytes.Buffer)
	buildCmd.Stderr = buildOutput

	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build binary: %v\nOutput: %s", err, buildOutput.String())
	}

	// Verify the binary was created.
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("binary not found at %s: %v", binaryPath, err)
	}

	// Run the binary with the "run" subcommand.
	runCmd := exec.Command(binaryPath, "run") //nolint:noctx
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	runCmd.Stdout = outBuf
	runCmd.Stderr = errBuf

	err := runCmd.Run()

	// Assert non-zero exit code.
	if err == nil {
		t.Fatal("expected non-zero exit code when running 'go-apply run', but got nil")
	}

	// Assert "unknown command" in output (cobra's standard message).
	combined := outBuf.String() + errBuf.String()
	if !strings.Contains(combined, "unknown command") {
		t.Errorf("expected 'unknown command' in output, got stdout=%q stderr=%q", outBuf.String(), errBuf.String())
	}
}

// TestRunCommandHelp asserts that the root command help does not list "run"
// after it has been removed. This test is also RED until T008/T009 delete the command.
func TestRunCommandHelp(t *testing.T) {
	outBuf := new(bytes.Buffer)
	root := cli.NewRootCommand("test")
	root.SetOut(outBuf)
	root.SetArgs([]string{"--help"})

	// Execute help command (should not error).
	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error running help: %v", err)
	}

	helpOutput := outBuf.String()
	// After removal, "run" should not appear in the help text as a command.
	// (Note: it might appear in descriptions, but not in the Commands section.)
	// This is a secondary assertion to verify the command is truly gone.
	// We'll check that "run" doesn't appear as "  run   " (command listing format).
	if strings.Contains(helpOutput, "  run   ") || strings.Contains(helpOutput, "  run\t") {
		t.Errorf("help output still lists 'run' command after removal:\n%s", helpOutput)
	}
}
