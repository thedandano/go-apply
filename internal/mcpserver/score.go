package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	mcppres "github.com/thedandano/go-apply/internal/presenter/mcp"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

// loadDeps loads configuration and wires all pipeline dependencies.
// Config is loaded fresh per invocation so changes take effect immediately.
// By default Claude is the orchestrator in MCP mode — LLM, CLGen, Augment, and
// Tailor are nil so Claude handles keyword extraction, cover letters, etc.
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
		log.Info("no orchestrator LLM — Claude handles keyword extraction in MCP mode")
	}

	deps := pipeline.ApplyConfig{
		Fetcher:  fetcherSvc,
		LLM:      llmClient,
		Scorer:   scorerSvc,
		CLGen:    nil, // Claude generates cover letters
		Resumes:  resumeRepo,
		Loader:   docLoader,
		AppRepo:  appRepo,
		Augment:  nil, // augment requires LLM to incorporate chunks — skipped in MCP mode
		Defaults: defaults,
		Tailor:   nil, // Claude handles tailoring
		// Presenter is set per-invocation inside each handler.
	}

	return cfg, deps, nil
}

// HandleGetScore is the exported handler for "get_score" tool calls.
// Presenter is assigned internally — callers must leave ApplyConfig.Presenter nil.
// This function never returns a Go error; all failures become JSON error responses.
func HandleGetScore(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig) *mcp.CallToolResult {
	return HandleGetScoreWithConfig(ctx, req, deps, nil)
}

// HandleGetScoreWithConfig is the full handler with optional *config.Config.
// When cfg is nil (tests), a zero-value config is used for non-nil fields.
func HandleGetScoreWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig, cfg *config.Config) *mcp.CallToolResult {
	jdURL := req.GetString("url", "")
	jdRawText := req.GetString("text", "")
	channelVal := req.GetString("channel", "COLD")
	accomplishmentsVal := req.GetString("accomplishments", "")

	if jdURL != "" && jdRawText != "" {
		return errorResult("exactly one of url or text is required")
	}
	if jdURL == "" && jdRawText == "" {
		return errorResult("exactly one of url or text is required")
	}

	channel, err := model.ParseChannel(channelVal)
	if err != nil {
		return errorResult(err.Error())
	}

	pres := mcppres.New()
	deps.Presenter = pres

	pl := pipeline.NewApplyPipeline(deps)

	isText := jdRawText != ""
	input := jdURL
	if isText {
		input = jdRawText
	}

	runErr := pl.Run(ctx, pipeline.ApplyRequest{
		URLOrText:           input,
		IsText:              isText,
		Channel:             channel,
		Config:              resolveConfig(cfg),
		AccomplishmentsText: accomplishmentsVal,
	})
	// If the pipeline errored but the presenter captured a structured result
	// (status "error" with a message), prefer that over a plain-text error —
	// it gives the MCP host actionable JSON rather than an opaque string.
	if runErr != nil && pres.Result == nil {
		return errorResult(runErr.Error())
	}

	if pres.Result == nil {
		return errorResult("pipeline produced no result")
	}

	data, err := json.Marshal(pres.Result)
	if err != nil {
		return errorResult(fmt.Sprintf("marshal result: %v", err))
	}
	return mcp.NewToolResultText(string(data))
}

// errorResult wraps an error message as a JSON text tool result.
// Returns the text representation of {"error": "<message>"}.
func errorResult(message string) *mcp.CallToolResult {
	data, _ := json.Marshal(map[string]string{"error": message})
	return mcp.NewToolResultText(string(data))
}

// resolveConfig returns cfg if non-nil, or a zero-value *config.Config for tests.
func resolveConfig(cfg *config.Config) *config.Config {
	if cfg != nil {
		return cfg
	}
	return &config.Config{}
}
