package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func ConfigDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "go-apply")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "go-apply")
}

func DataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "go-apply")
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "share", "go-apply")
}

// LogDir returns the XDG_STATE_HOME log directory.
// XDG spec: state files (logs, history) → ~/.local/state
// Full path: ~/.local/state/go-apply/logs/
func LogDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "go-apply", "logs")
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "state", "go-apply", "logs")
}

type LLMProviderConfig struct {
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
	APIKey  string `yaml:"api_key"`
}

// Config is the top-level application configuration.
//
// Two independent LLM providers:
//
//	orchestrator: heavy reasoning model (keyword extraction, cover letter, bullet rewrites)
//	embedder:     embedding model (vector search for semantic resume↔JD matching)
//
// Example config.yaml:
//
//	orchestrator:
//	  base_url: https://api.anthropic.com/v1
//	  model: claude-sonnet-4-6
//	  api_key: sk-ant-...
//	embedder:
//	  base_url: http://localhost:11434/v1
//	  model: nomic-embed-text
//	  api_key: ""
//	embedding_dim: 768
type Config struct {
	Orchestrator      LLMProviderConfig `yaml:"orchestrator"`
	Embedder          LLMProviderConfig `yaml:"embedder"`
	EmbeddingDim      int               `yaml:"embedding_dim"`
	DBPath            string            `yaml:"db_path"`
	LogLevel          string            `yaml:"log_level"`
	DefaultSeniority  string            `yaml:"default_seniority"`
	UserName          string            `yaml:"user_name"`
	Occupation        string            `yaml:"occupation"`
	Location          string            `yaml:"location"`
	LinkedInURL       string            `yaml:"linkedin_url"`
	YearsOfExperience float64           `yaml:"years_of_experience"`
}

func (c *Config) ResolveLogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (c *Config) ResolveEmbeddingDim() int {
	if c.EmbeddingDim > 0 {
		return c.EmbeddingDim
	}
	return EmbeddedDefaults().VectorSearch.DefaultEmbeddingDim
}

func (c *Config) ResolveDBPath() string {
	if c.DBPath != "" {
		return c.DBPath
	}
	return filepath.Join(DataDir(), "profile.db")
}

func Load() (*Config, error) {
	cfg := &Config{}
	cfgFile := filepath.Join(ConfigDir(), "config.yaml")
	data, err := os.ReadFile(cfgFile) // #nosec G304 -- config file path is user-controlled XDG path
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config %s: %w", cfgFile, err)
	}
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config %s: %w", cfgFile, err)
		}
	}
	if key := os.Getenv("GO_APPLY_API_KEY"); key != "" {
		cfg.Orchestrator.APIKey = key
	}
	return cfg, nil
}

func (c *Config) Save() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, data, 0600); err != nil { // #nosec G306 -- config file, user-owned
		return fmt.Errorf("write config %s: %w", cfgPath, err)
	}
	return nil
}
