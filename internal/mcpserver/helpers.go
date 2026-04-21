package mcpserver

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/redact"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/repository/sqlite"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

// newSQLiteProfile opens the SQLite profile/keyword-cache database.
// Returns a concrete *sqlite.ProfileRepository because it satisfies both
// port.ProfileRepository and port.KeywordCacheRepository, so callers can pass
// it for both parameters without an additional type assertion.
func newSQLiteProfile(cfg *config.Config) (*sqlite.ProfileRepository, error) {
	repo, err := sqlite.NewProfileRepository(cfg.ResolveDBPath(), cfg.ResolveEmbeddingDim())
	if err != nil {
		return nil, fmt.Errorf("open profile db %s: %w", cfg.ResolveDBPath(), err)
	}
	return repo, nil
}

// loadDeps loads configuration and wires all pipeline dependencies.
// Config is loaded fresh per invocation so changes take effect immediately.
// The MCP host is the orchestrator — LLM, CLGen, and Tailor are nil so the host
// handles keyword extraction, cover letters, and tailoring.
func loadDeps() (*config.Config, pipeline.ApplyConfig, error) {
	log := slog.Default()

	cfg, err := config.Load()
	if err != nil {
		return nil, pipeline.ApplyConfig{}, fmt.Errorf("load config: %w", err)
	}

	defaults, err := config.LoadDefaults()
	if err != nil {
		return nil, pipeline.ApplyConfig{}, fmt.Errorf("load defaults: %w", err)
	}

	dataDir := config.DataDir()
	appRepo := fs.NewApplicationRepository(dataDir)
	resumeRepo := fs.NewResumeRepository(dataDir)
	docLoader := loader.New()

	scorerSvc := scorer.New(defaults)
	fetcherSvc := fetcher.NewFallback(defaults, log)

	deps := pipeline.ApplyConfig{
		Fetcher:  fetcherSvc,
		LLM:      nil,
		Scorer:   scorerSvc,
		CLGen:    nil,
		Resumes:  resumeRepo,
		Loader:   docLoader,
		AppRepo:  appRepo,
		Defaults: defaults,
		Tailor:   nil,
	}

	r := redact.New(&redact.Profile{
		Name:        cfg.UserName,
		Location:    cfg.Location,
		LinkedInURL: cfg.LinkedInURL,
	})
	logger.SetRedactor(r)

	return cfg, deps, nil
}

// errorResult wraps an error message as a JSON text tool result.
func errorResult(message string) *mcp.CallToolResult {
	data, _ := json.Marshal(map[string]string{"error": message})
	return mcp.NewToolResultText(string(data))
}
