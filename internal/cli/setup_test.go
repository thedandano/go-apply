package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
)

// executeSetupMCP runs the root command with the given args and returns
// (stdout, stderr, error). It redirects both output streams so tests can
// inspect them without touching real agent config files.
func executeSetupMCP(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := cli.NewRootCommand("test")
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// TestSetupMCP_MissingAgentFlag verifies that omitting --agent returns an error.
func TestSetupMCP_MissingAgentFlag(t *testing.T) {
	_, _, err := executeSetupMCP(t, "setup", "mcp")
	if err == nil {
		t.Fatal("expected error when --agent flag is missing, got nil")
	}
}

// TestSetupMCP_UnknownAgent verifies that an unrecognised agent name surfaces
// an error message that mentions "valid agents".
func TestSetupMCP_UnknownAgent(t *testing.T) {
	_, errOut, err := executeSetupMCP(t, "setup", "mcp", "--agent", "foo")
	if err == nil {
		t.Fatal("expected error for unknown agent, got nil")
	}
	combined := err.Error() + errOut
	if !strings.Contains(combined, "valid agents") {
		t.Errorf("expected error to contain \"valid agents\", got:\nerr=%v\nstderr=%s", err, errOut)
	}
}

// TestSetupMCP_Register_CreatesFile verifies that running setup mcp --agent hermes
// creates config.yaml in an otherwise empty HERMES_HOME directory.
func TestSetupMCP_Register_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)

	stdout, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Created") {
		t.Errorf("expected output to contain \"Created\", got: %q", stdout)
	}

	cfgPath := filepath.Join(tmpDir, "config.yaml")
	data, readErr := os.ReadFile(cfgPath)
	if readErr != nil {
		t.Fatalf("config.yaml not created: %v", readErr)
	}
	if !strings.Contains(string(data), "go-apply") {
		t.Errorf("config.yaml does not contain go-apply entry:\n%s", data)
	}
	if !strings.Contains(string(data), "serve") {
		t.Errorf("config.yaml does not contain 'serve' arg:\n%s", data)
	}
}

// TestSetupMCP_Register_Idempotent verifies that running setup mcp twice produces
// "already registered" on the second run and leaves the file unchanged.
func TestSetupMCP_Register_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)

	if _, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes"); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	cfgPath := filepath.Join(tmpDir, "config.yaml")
	first, _ := os.ReadFile(cfgPath)

	stdout, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes")
	if err != nil {
		t.Fatalf("second registration returned error: %v", err)
	}
	if !strings.Contains(stdout, "already registered") {
		t.Errorf("expected \"already registered\" on second run, got: %q", stdout)
	}

	second, _ := os.ReadFile(cfgPath)
	if !bytes.Equal(first, second) {
		t.Errorf("config file changed on second run:\nbefore:\n%s\nafter:\n%s", first, second)
	}
}

// TestSetupMCP_Register_PreservesExistingServers verifies that registering
// go-apply does not remove another server that was already in the config file.
func TestSetupMCP_Register_PreservesExistingServers(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)

	existing := `mcp_servers:
  other-tool:
    command: /usr/local/bin/other-tool
    args:
      - --run
`
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	if _, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes"); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	data, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(data), "other-tool") {
		t.Errorf("other-tool was removed from config:\n%s", data)
	}
	if !strings.Contains(string(data), "go-apply") {
		t.Errorf("go-apply entry not present:\n%s", data)
	}
}

// TestSetupMCP_Remove_RemovesEntry registers go-apply then removes it, verifying
// that the output says "Removed" and the entry is gone from the config file.
func TestSetupMCP_Remove_RemovesEntry(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)

	if _, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes"); err != nil {
		t.Fatalf("registration failed: %v", err)
	}

	stdout, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes", "--remove")
	if err != nil {
		t.Fatalf("remove returned error: %v", err)
	}
	if !strings.Contains(stdout, "Removed") {
		t.Errorf("expected output to contain \"Removed\", got: %q", stdout)
	}

	data, _ := os.ReadFile(filepath.Join(tmpDir, "config.yaml"))
	if strings.Contains(string(data), "go-apply") {
		t.Errorf("go-apply entry still present after removal:\n%s", data)
	}
}

// TestSetupMCP_Remove_NotRegistered verifies that removing a non-existent entry
// exits with code 0 and reports "not found".
func TestSetupMCP_Remove_NotRegistered(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)

	// Write an existing config without go-apply.
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("mcp_servers: {}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stdout, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes", "--remove")
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	if !strings.Contains(stdout, "not found") {
		t.Errorf("expected \"not found\" in output, got: %q", stdout)
	}
}

// TestSetupMCP_Remove_ConfigMissing verifies that removing when no config file
// exists exits with code 0 and reports "not found".
func TestSetupMCP_Remove_ConfigMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)

	// No config.yaml written — directory is empty.
	stdout, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes", "--remove")
	if err != nil {
		t.Fatalf("expected exit 0 when config missing, got error: %v", err)
	}
	if !strings.Contains(stdout, "not found") {
		t.Errorf("expected \"not found\" in output, got: %q", stdout)
	}
}

// TestSetupMCP_Override_CallsRegisterForce verifies that --override (or --force)
// overwrites an existing entry instead of returning "already registered".
func TestSetupMCP_Override_CallsRegisterForce(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)

	// First registration.
	if _, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes"); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Second registration with --override should succeed without "already registered".
	stdout, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes", "--override")
	if err != nil {
		t.Fatalf("--override registration returned error: %v", err)
	}
	if strings.Contains(stdout, "already registered") {
		t.Errorf("--override should not produce 'already registered', got: %q", stdout)
	}
}

// TestSetupMCP_Force_AliasForOverride verifies that --force is an alias for --override.
func TestSetupMCP_Force_AliasForOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)

	// First registration.
	if _, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes"); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Second registration with --force.
	stdout, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes", "--force")
	if err != nil {
		t.Fatalf("--force registration returned error: %v", err)
	}
	if strings.Contains(stdout, "already registered") {
		t.Errorf("--force should not produce 'already registered', got: %q", stdout)
	}
}

// TestSetupMCP_NonTTY_AlreadyRegistered verifies that without --override in a
// non-TTY context (all tests run non-TTY), "already registered" is printed but
// no error is returned (exit 0).
func TestSetupMCP_NonTTY_AlreadyRegistered(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)

	if _, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes"); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	stdout, _, err := executeSetupMCP(t, "setup", "mcp", "--agent", "hermes")
	if err != nil {
		t.Fatalf("expected exit 0 for already-registered in non-TTY, got: %v", err)
	}
	if !strings.Contains(stdout, "already registered") {
		t.Errorf("expected 'already registered' in stdout, got: %q", stdout)
	}
}

// TestSetupMCP_AgentAll_RegistersAllAgents verifies that --agent all iterates
// over all three agents and produces a status line per agent.
func TestSetupMCP_AgentAll_RegistersAllAgents(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)
	t.Setenv("OPENCLAW_CONFIG_PATH", filepath.Join(tmpDir, "openclaw.json"))

	stdout, _, _ := executeSetupMCP(t, "setup", "mcp", "--agent", "all")
	if !strings.Contains(stdout, "hermes:") {
		t.Errorf("expected hermes status line in --agent all output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "openclaw:") {
		t.Errorf("expected openclaw status line in --agent all output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "claude:") {
		t.Errorf("expected claude status line in --agent all output, got: %q", stdout)
	}
}

// TestSetupMCP_AgentAll_Remove verifies that --remove --agent all iterates over
// all three agents and produces a status line per agent.
func TestSetupMCP_AgentAll_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HERMES_HOME", tmpDir)
	t.Setenv("OPENCLAW_CONFIG_PATH", filepath.Join(tmpDir, "openclaw.json"))

	stdout, _, _ := executeSetupMCP(t, "setup", "mcp", "--agent", "all", "--remove")
	if !strings.Contains(stdout, "hermes:") {
		t.Errorf("expected hermes status line in --remove --agent all output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "openclaw:") {
		t.Errorf("expected openclaw status line in --remove --agent all output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "claude:") {
		t.Errorf("expected claude status line in --remove --agent all output, got: %q", stdout)
	}
}
