package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/repository/sqlite"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/llm"
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
// By default the MCP host is the orchestrator in MCP mode — LLM, CLGen, Augment, and
// Tailor are nil so the MCP host handles keyword extraction, cover letters, etc.
// When the orchestrator section is configured (base_url + model), an LLM client
// is created so the pipeline can extract keywords autonomously.
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

	var llmClient port.LLMClient
	if cfg.Orchestrator.BaseURL != "" && cfg.Orchestrator.Model != "" {
		log.Info("orchestrator LLM configured — pipeline will extract keywords", "model", cfg.Orchestrator.Model)
		llmClient = llm.New(cfg.Orchestrator.BaseURL, cfg.Orchestrator.Model, cfg.Orchestrator.APIKey, defaults, log)
	} else {
		logger.Decision(context.Background(), log, "keyword_extraction", "mcp_host", "no orchestrator LLM configured")
	}

	deps := pipeline.ApplyConfig{
		Fetcher:  fetcherSvc,
		LLM:      llmClient,
		Scorer:   scorerSvc,
		CLGen:    nil,
		Resumes:  resumeRepo,
		Loader:   docLoader,
		AppRepo:  appRepo,
		Augment:  nil,
		Defaults: defaults,
		Tailor:   nil,
	}

	return cfg, deps, nil
}

// errorResult wraps an error message as a JSON text tool result.
func errorResult(message string) *mcp.CallToolResult {
	data, _ := json.Marshal(map[string]string{"error": message})
	return mcp.NewToolResultText(string(data))
}
