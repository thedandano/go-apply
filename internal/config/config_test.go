package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
)

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `
years_of_experience: 7.0
`
	cfgDir := filepath.Join(dir, "go-apply")
	if err := os.MkdirAll(cfgDir, config.DirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(yaml), config.FilePerm); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.YearsOfExperience != 7.0 {
		t.Errorf("YearsOfExperience = %f, want 7.0", cfg.YearsOfExperience)
	}
}

// TestLoadAutoCreatesConfigWhenMissing verifies that Load() creates a zero-value
// config.yaml when none exists, so first-run works without a manual init step.
func TestLoadAutoCreatesConfigWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() should not error on missing config, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	// Config file should now exist on disk.
	cfgPath := filepath.Join(dir, "go-apply", "config.yaml")
	if _, statErr := os.Stat(cfgPath); os.IsNotExist(statErr) {
		t.Errorf("config file not created at %s", cfgPath)
	}

	// Second Load() should read the persisted file without error.
	cfg2, err := config.Load()
	if err != nil {
		t.Fatalf("second Load() error: %v", err)
	}
	if cfg2 == nil {
		t.Fatal("second Load() returned nil config")
	}
}

func TestResolveLogLevel(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"debug", "DEBUG"},
		{"DEBUG", "DEBUG"},
		{"warn", "WARN"},
		{"error", "ERROR"},
		{"", "INFO"},
		{"invalid", "INFO"},
	}
	for _, tc := range cases {
		cfg := &config.Config{LogLevel: tc.input}
		got := cfg.ResolveLogLevel().String()
		if got != tc.want {
			t.Errorf("ResolveLogLevel(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
