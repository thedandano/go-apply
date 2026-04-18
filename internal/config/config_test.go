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
orchestrator:
  base_url: "https://api.example.com/v1"
  model: "test-model"
  api_key: "key-from-file"
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
	if err := os.MkdirAll(cfgDir, config.DirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(yaml), config.FilePerm); err != nil {
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

// TestLoadAutoCreatesConfigWhenMissing verifies that Load() creates a zero-value
// config.yaml when none exists, so first-run works without a manual init step.
func TestLoadAutoCreatesConfigWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("GO_APPLY_API_KEY", "")

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

func TestApplyOverrides_AllFields(t *testing.T) {
	defaults := config.EmbeddedDefaults()
	cfg := &config.Config{
		Thresholds: config.ThresholdOverrides{ScorePass: 80, ScoreBoostMin: 50},
		Timeouts:   config.TimeoutOverrides{LLMMS: 120000, FetcherMS: 90000},
	}
	cfg.ApplyOverrides(defaults)
	if defaults.Thresholds.ScorePass != 80 {
		t.Errorf("ScorePass = %f, want 80", defaults.Thresholds.ScorePass)
	}
	if defaults.Thresholds.ScoreBoostMin != 50 {
		t.Errorf("ScoreBoostMin = %f, want 50", defaults.Thresholds.ScoreBoostMin)
	}
	if defaults.LLM.TimeoutMS != 120000 {
		t.Errorf("LLM.TimeoutMS = %d, want 120000", defaults.LLM.TimeoutMS)
	}
	if defaults.Fetcher.ChromedpTimeoutMS != 90000 {
		t.Errorf("Fetcher.ChromedpTimeoutMS = %d, want 90000", defaults.Fetcher.ChromedpTimeoutMS)
	}
}

func TestApplyOverrides_ZeroFields_DoNotOverride(t *testing.T) {
	defaults := config.EmbeddedDefaults()
	wantScorePass := defaults.Thresholds.ScorePass
	wantLLMMS := defaults.LLM.TimeoutMS
	cfg := &config.Config{} // zero overrides
	cfg.ApplyOverrides(defaults)
	if defaults.Thresholds.ScorePass != wantScorePass {
		t.Errorf("ScorePass changed: got %f, want %f", defaults.Thresholds.ScorePass, wantScorePass)
	}
	if defaults.LLM.TimeoutMS != wantLLMMS {
		t.Errorf("LLM.TimeoutMS changed: got %d, want %d", defaults.LLM.TimeoutMS, wantLLMMS)
	}
}

func TestApplyOverrides_Partial(t *testing.T) {
	defaults := config.EmbeddedDefaults()
	origBoostMin := defaults.Thresholds.ScoreBoostMin
	cfg := &config.Config{
		Thresholds: config.ThresholdOverrides{ScorePass: 75}, // only ScorePass set
	}
	cfg.ApplyOverrides(defaults)
	if defaults.Thresholds.ScorePass != 75 {
		t.Errorf("ScorePass = %f, want 75", defaults.Thresholds.ScorePass)
	}
	if defaults.Thresholds.ScoreBoostMin != origBoostMin {
		t.Errorf("ScoreBoostMin changed unexpectedly: got %f", defaults.Thresholds.ScoreBoostMin)
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

func TestValidateCLI_MissingBaseURL_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		Orchestrator: config.LLMProviderConfig{Model: "some-model"},
	}
	if err := cfg.ValidateCLI(); err == nil {
		t.Error("ValidateCLI() should return error when base_url is empty")
	}
}

func TestValidateCLI_MissingModel_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		Orchestrator: config.LLMProviderConfig{BaseURL: "https://api.example.com/v1"},
	}
	if err := cfg.ValidateCLI(); err == nil {
		t.Error("ValidateCLI() should return error when model is empty")
	}
}

func TestValidateCLI_Valid_ReturnsNil(t *testing.T) {
	cfg := &config.Config{
		Orchestrator: config.LLMProviderConfig{
			BaseURL: "https://api.example.com/v1",
			Model:   "some-model",
		},
	}
	if err := cfg.ValidateCLI(); err != nil {
		t.Errorf("ValidateCLI() unexpected error: %v", err)
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
