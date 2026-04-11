package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
)

// TestDefaultsMatchJSON verifies EmbeddedDefaults() matches internal/config/defaults.json.
// Fails CI if someone edits one and not the other.
func TestDefaultsMatchJSON(t *testing.T) {
	fromFile, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults() failed: %v", err)
	}
	embedded := config.EmbeddedDefaults()
	if !reflect.DeepEqual(fromFile, embedded) {
		t.Errorf("defaults.json and EmbeddedDefaults() are out of sync.\nJSON: %+v\nEmbedded: %+v", fromFile, embedded)
	}
}

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `
orchestrator:
  base_url: "https://api.example.com/v1"
  model: "test-model"
  api_key: "key-from-file"
years_of_experience: 7.0
`
	cfgDir := filepath.Join(dir, "go-apply")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Orchestrator.BaseURL != "https://api.example.com/v1" {
		t.Errorf("BaseURL = %q, want %q", cfg.Orchestrator.BaseURL, "https://api.example.com/v1")
	}
	if cfg.YearsOfExperience != 7.0 {
		t.Errorf("YearsOfExperience = %f, want 7.0", cfg.YearsOfExperience)
	}
}

func TestEnvVarOverridesAPIKey(t *testing.T) {
	dir := t.TempDir()
	yaml := `orchestrator:
  api_key: "key-from-file"`
	cfgDir := filepath.Join(dir, "go-apply")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("GO_APPLY_API_KEY", "key-from-env")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Orchestrator.APIKey != "key-from-env" {
		t.Errorf("APIKey = %q, want %q", cfg.Orchestrator.APIKey, "key-from-env")
	}
}

func TestLoadReturnsDefaultsWhenNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("GO_APPLY_API_KEY", "")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("cfg is nil")
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

func TestResolveEmbeddingDim(t *testing.T) {
	wantDefault := config.EmbeddedDefaults().VectorSearch.DefaultEmbeddingDim
	cfg := &config.Config{}
	if got := cfg.ResolveEmbeddingDim(); got != wantDefault {
		t.Errorf("default dim = %d, want %d", got, wantDefault)
	}
	cfg.EmbeddingDim = 768
	if got := cfg.ResolveEmbeddingDim(); got != 768 {
		t.Errorf("custom dim = %d, want 768", got)
	}
}

func TestResolveDBPath(t *testing.T) {
	cfg := &config.Config{DBPath: "/custom/path/db"}
	if got := cfg.ResolveDBPath(); got != "/custom/path/db" {
		t.Errorf("custom db path = %q, want /custom/path/db", got)
	}
	cfg2 := &config.Config{}
	if got := cfg2.ResolveDBPath(); got == "" {
		t.Error("default db path should not be empty")
	}
}
