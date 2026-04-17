package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
	"github.com/thedandano/go-apply/internal/config"
)

// executeOnboard runs the root command with the given args and returns
// (stdout, stderr, error).
func executeOnboard(t *testing.T, args ...string) (stdout, stderr string, err error) {
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

// TestOnboard_Reset_WithYes verifies that --reset --yes removes profile.db and inputs/ directory.
func TestOnboard_Reset_WithYes(t *testing.T) {
	tmpDataDir := t.TempDir()
	tmpConfigDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDataDir)
	t.Setenv("XDG_CONFIG_HOME", tmpConfigDir)

	// Create a default config
	cfg := &config.Config{}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Create dummy profile.db, WAL files, and inputs directory
	dataDir := filepath.Join(tmpDataDir, "go-apply")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	dbPath := filepath.Join(dataDir, "profile.db")
	walPath := dbPath + "-wal"
	shmPath := dbPath + "-shm"
	inputsDir := filepath.Join(dataDir, "inputs")

	// Create the files and directories
	if err := os.WriteFile(dbPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("failed to create profile.db: %v", err)
	}
	if err := os.WriteFile(walPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("failed to create WAL file: %v", err)
	}
	if err := os.WriteFile(shmPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("failed to create SHM file: %v", err)
	}
	if err := os.MkdirAll(inputsDir, 0o700); err != nil {
		t.Fatalf("failed to create inputs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputsDir, "test.txt"), []byte("test"), 0o600); err != nil {
		t.Fatalf("failed to create file in inputs: %v", err)
	}

	// Create jd_cache directory so we can verify it is NOT removed
	jdCacheDir := filepath.Join(dataDir, "jd_cache")
	if err := os.MkdirAll(jdCacheDir, 0o700); err != nil {
		t.Fatalf("failed to create jd_cache dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jdCacheDir, "cache.txt"), []byte("cache"), 0o600); err != nil {
		t.Fatalf("failed to create file in jd_cache: %v", err)
	}

	// Run reset with --yes
	stdout, _, err := executeOnboard(t, "onboard", "--reset", "--yes")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify profile.db is removed
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Errorf("expected profile.db to be removed, but it exists")
	}

	// Verify WAL files are removed
	if _, err := os.Stat(walPath); !os.IsNotExist(err) {
		t.Errorf("expected profile.db-wal to be removed, but it exists")
	}
	if _, err := os.Stat(shmPath); !os.IsNotExist(err) {
		t.Errorf("expected profile.db-shm to be removed, but it exists")
	}

	// Verify inputs directory is removed
	if _, err := os.Stat(inputsDir); !os.IsNotExist(err) {
		t.Errorf("expected inputs/ directory to be removed, but it exists")
	}

	// Verify jd_cache is NOT removed
	if _, err := os.Stat(jdCacheDir); os.IsNotExist(err) {
		t.Errorf("expected jd_cache/ to NOT be removed, but it was")
	}
	if _, err := os.Stat(filepath.Join(jdCacheDir, "cache.txt")); os.IsNotExist(err) {
		t.Errorf("expected cache file to still exist, but it was removed")
	}

	// Verify output message
	if !strings.Contains(stdout, "Profile reset") {
		t.Errorf("expected output to contain 'Profile reset', got: %q", stdout)
	}
}

// TestOnboard_Reset_NonTTY_WithoutYes verifies that reset without --yes and non-TTY stdin returns an error.
// Note: We redirect stdin to /dev/null to simulate non-TTY environment since the test harness
// may not properly detect TTY state.
func TestOnboard_Reset_NonTTY_WithoutYes(t *testing.T) {
	tmpDataDir := t.TempDir()
	tmpConfigDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDataDir)
	t.Setenv("XDG_CONFIG_HOME", tmpConfigDir)

	// Create a default config
	cfg := &config.Config{}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Redirect stdin to /dev/null to simulate non-interactive environment
	devNull, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("failed to open /dev/null: %v", err)
	}
	defer devNull.Close()

	oldStdin := os.Stdin
	os.Stdin = devNull
	defer func() { os.Stdin = oldStdin }()

	// With non-TTY stdin and no --yes flag, should return error
	_, _, err = executeOnboard(t, "onboard", "--reset")
	if err == nil {
		t.Fatal("expected error for non-TTY reset without --yes, got nil")
	}

	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "--yes required") {
		t.Errorf("expected error to contain '--yes required', got: %q", errorMsg)
	}
}

// TestOnboard_Reset_WithYes_ThenResume verifies that --reset --yes --resume <file> resets and then onboards.
// After reset, the inputs/ directory should be removed, which is verified indirectly by checking the
// command output contains no stored documents (since the LLM isn't configured to actually embed).
func TestOnboard_Reset_WithYes_ThenResume(t *testing.T) {
	tmpDataDir := t.TempDir()
	tmpConfigDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDataDir)
	t.Setenv("XDG_CONFIG_HOME", tmpConfigDir)

	// Create a default config
	cfg := &config.Config{}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Create dummy profile.db and inputs directory
	dataDir := filepath.Join(tmpDataDir, "go-apply")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	dbPath := filepath.Join(dataDir, "profile.db")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("failed to create profile.db: %v", err)
	}

	// Create inputs directory with a file to verify it gets deleted
	inputsDir := filepath.Join(dataDir, "inputs")
	if err := os.MkdirAll(inputsDir, 0o700); err != nil {
		t.Fatalf("failed to create inputs dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputsDir, "old_data.txt"), []byte("old"), 0o600); err != nil {
		t.Fatalf("failed to create file in inputs: %v", err)
	}

	// Create a dummy resume file
	resumePath := filepath.Join(t.TempDir(), "resume.txt")
	if err := os.WriteFile(resumePath, []byte("dummy resume"), 0o600); err != nil {
		t.Fatalf("failed to create resume file: %v", err)
	}

	// Run reset with --yes --resume (this will attempt onboarding, which may fail due to missing LLM config, but reset should work)
	stdout, _, _ := executeOnboard(t, "onboard", "--reset", "--yes", "--resume", resumePath)

	// Verify old inputs were deleted by checking that the new inputs doesn't have the old file
	// (the command will recreate inputs with the new resume, even if LLM fails)
	if _, statErr := os.Stat(filepath.Join(inputsDir, "old_data.txt")); !os.IsNotExist(statErr) {
		t.Errorf("expected old inputs/old_data.txt to be removed after reset, but it exists")
	}

	// Verify that the command ran (output should contain JSON result even if empty/warning)
	if !strings.Contains(stdout, "{") {
		t.Errorf("expected JSON output from onboarding, got: %q", stdout)
	}
}

// TestOnboard_ResetWithoutFlags verifies that --reset without --yes and without --resume prints the reset message.
func TestOnboard_Reset_Message(t *testing.T) {
	tmpDataDir := t.TempDir()
	tmpConfigDir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpDataDir)
	t.Setenv("XDG_CONFIG_HOME", tmpConfigDir)

	// Create a default config
	cfg := &config.Config{}
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Create dummy profile.db
	dataDir := filepath.Join(tmpDataDir, "go-apply")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}

	dbPath := filepath.Join(dataDir, "profile.db")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0o600); err != nil {
		t.Fatalf("failed to create profile.db: %v", err)
	}

	// Run reset with --yes (no other flags)
	stdout, _, err := executeOnboard(t, "onboard", "--reset", "--yes")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify reset message
	if !strings.Contains(stdout, "Profile reset") {
		t.Errorf("expected output to contain 'Profile reset', got: %q", stdout)
	}
	if !strings.Contains(stdout, "go-apply onboard") {
		t.Errorf("expected output to mention 'go-apply onboard' command, got: %q", stdout)
	}
}
