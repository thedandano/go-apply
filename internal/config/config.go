package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// DirPerm and FilePerm are the Unix permission bits used for all config
// directories and files. Exported so callers and tests use one source of truth.
const (
	DirPerm  fs.FileMode = 0o700 // config directory: owner rwx, no group/other access
	FilePerm fs.FileMode = 0o600 // config file: owner rw-, no group/other access
)

func Dir() string {
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

// StateDir returns the XDG_STATE_HOME directory for go-apply.
// XDG spec: state files (logs, history, caches) → ~/.local/state
func StateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "go-apply")
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "state", "go-apply")
}

// LogDir returns the XDG_STATE_HOME log directory.
// Full path: ~/.local/state/go-apply/logs/
func LogDir() string {
	return filepath.Join(StateDir(), "logs")
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

// Load reads config.yaml from the XDG config directory.
// Returns an error if the file does not exist or cannot be parsed —
// a missing config file is not a valid state; use `go-apply init` to create one.
// The GO_APPLY_API_KEY environment variable overrides the orchestrator API key.
func Load() (*Config, error) {
	cfgFile := filepath.Join(Dir(), "config.yaml")
	slog.Debug("loading config", "path", cfgFile)

	data, err := os.ReadFile(cfgFile) // #nosec G304 -- path is XDG_CONFIG_HOME/go-apply/config.yaml, not user input
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			cfg := &Config{}
			if saveErr := cfg.Save(); saveErr != nil {
				return nil, fmt.Errorf("create default config at %s: %w", cfgFile, saveErr)
			}
			slog.Info("created default config", "path", cfgFile)
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", cfgFile, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", cfgFile, err)
	}
	slog.Info("config loaded", "path", cfgFile, "log_level", cfg.LogLevel, "model", cfg.Orchestrator.Model)

	if key := os.Getenv("GO_APPLY_API_KEY"); key != "" {
		slog.Debug("orchestrator API key overridden by GO_APPLY_API_KEY env var")
		cfg.Orchestrator.APIKey = key
	}

	return cfg, nil
}

// ValidateCLI returns an error if the orchestrator config required for CLI/TUI
// mode is incomplete. MCP mode does not call this — the MCP host is the orchestrator.
func (c *Config) ValidateCLI() error {
	if strings.TrimSpace(c.Orchestrator.BaseURL) == "" {
		return fmt.Errorf("orchestrator.base_url is not set — edit %s or set GO_APPLY_API_KEY", filepath.Join(Dir(), "config.yaml"))
	}
	if strings.TrimSpace(c.Orchestrator.Model) == "" {
		return fmt.Errorf("orchestrator.model is not set — edit %s", filepath.Join(Dir(), "config.yaml"))
	}
	return nil
}

// AllKeys returns all user-facing dot-notation config keys in canonical order.
// Internal keys (db_path, log_level, default_seniority) are intentionally excluded;
// they can only be set by editing config.yaml directly.
func AllKeys() []string {
	return []string{
		"orchestrator.base_url",
		"orchestrator.model",
		"orchestrator.api_key",
		"embedder.base_url",
		"embedder.model",
		"embedder.api_key",
		"embedding_dim",
		"user_name",
		"occupation",
		"location",
		"linkedin_url",
		"years_of_experience",
	}
}

// MCPKeys returns the config keys relevant in MCP mode.
// Orchestrator keys are excluded: in MCP mode Claude (the MCP host) is the orchestrator
// and no separately-configured LLM is needed for reasoning tasks.
// Derived from AllKeys() by filtering orchestrator.* so new keys are automatically included.
func MCPKeys() []string {
	all := AllKeys()
	out := make([]string, 0, len(all))
	for _, k := range all {
		if !strings.HasPrefix(k, "orchestrator.") {
			out = append(out, k)
		}
	}
	return out
}

// IsAPIKey reports whether a dot-notation config key holds an API key value.
func IsAPIKey(key string) bool {
	return strings.HasSuffix(key, ".api_key")
}

// SetField sets a config field by dot-notation key, parsing the value string to the correct type.
func (c *Config) SetField(key, value string) error {
	switch key {
	case "orchestrator.base_url":
		c.Orchestrator.BaseURL = value
	case "orchestrator.model":
		c.Orchestrator.Model = value
	case "orchestrator.api_key":
		c.Orchestrator.APIKey = value
	case "embedder.base_url":
		c.Embedder.BaseURL = value
	case "embedder.model":
		c.Embedder.Model = value
	case "embedder.api_key":
		c.Embedder.APIKey = value
	case "embedding_dim":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("embedding_dim must be an integer: %w", err)
		}
		c.EmbeddingDim = n
	case "user_name":
		c.UserName = value
	case "occupation":
		c.Occupation = value
	case "location":
		c.Location = value
	case "linkedin_url":
		c.LinkedInURL = value
	case "years_of_experience":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("years_of_experience must be a number: %w", err)
		}
		c.YearsOfExperience = f
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

// GetField returns the string representation of a config field by dot-notation key.
func (c *Config) GetField(key string) (string, error) {
	switch key {
	case "orchestrator.base_url":
		return c.Orchestrator.BaseURL, nil
	case "orchestrator.model":
		return c.Orchestrator.Model, nil
	case "orchestrator.api_key":
		return c.Orchestrator.APIKey, nil
	case "embedder.base_url":
		return c.Embedder.BaseURL, nil
	case "embedder.model":
		return c.Embedder.Model, nil
	case "embedder.api_key":
		return c.Embedder.APIKey, nil
	case "embedding_dim":
		return strconv.Itoa(c.EmbeddingDim), nil
	case "user_name":
		return c.UserName, nil
	case "occupation":
		return c.Occupation, nil
	case "location":
		return c.Location, nil
	case "linkedin_url":
		return c.LinkedInURL, nil
	case "years_of_experience":
		return strconv.FormatFloat(c.YearsOfExperience, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

func (c *Config) Save() error {
	dir := Dir()
	if err := os.MkdirAll(dir, DirPerm); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, data, FilePerm); err != nil { // #nosec G306 -- config file, user-owned
		return fmt.Errorf("write config %s: %w", cfgPath, err)
	}
	slog.Info("config saved", "path", cfgPath)
	return nil
}
