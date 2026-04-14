// Package cli provides Cobra commands for the go-apply CLI.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	mcppres "github.com/thedandano/go-apply/internal/presenter/mcp"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/augment"
	"github.com/thedandano/go-apply/internal/service/coverletter"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
	tailor "github.com/thedandano/go-apply/internal/service/tailor"
)

// NewServeCommand returns the cobra command for "go-apply serve".
// It starts an MCP stdio server that exposes pipeline tools for Claude Code integration.
func NewServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP stdio server for Claude Code integration",
		RunE: func(_ *cobra.Command, _ []string) error {
			srv := server.NewMCPServer("go-apply", "0.1.0")

			srv.AddTool(
				mcp.NewTool("get_score",
					mcp.WithDescription("Score resumes against a job description. Runs the full pipeline; a cover letter is included in the result if the best score meets the configured threshold."),
					mcp.WithString("url", mcp.Description("URL of the job posting to fetch")),
					mcp.WithString("text", mcp.Description("Raw job description text (alternative to url)")),
					mcp.WithString("channel", mcp.Description("Application channel: COLD, REFERRAL, or RECRUITER"), mcp.DefaultString("COLD")),
					mcp.WithString("accomplishments", mcp.Description("Path to accomplishments doc for tier-2 bullet rewriting (optional)")),
				),
				func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					cfg, deps, err := loadDeps()
					if err != nil {
						return errorResult(fmt.Sprintf("load config: %v", err)), nil
					}
					return HandleGetScoreWithConfig(ctx, &req, &deps, cfg), nil
				},
			)

			return server.ServeStdio(srv)
		},
	}
}

// loadDeps loads configuration and wires all pipeline dependencies.
// Config is loaded fresh per invocation so changes take effect immediately.
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

	llmClient := llm.New(cfg.Orchestrator.BaseURL, cfg.Orchestrator.Model, cfg.Orchestrator.APIKey, defaults, log)
	embedderClient := llm.New(cfg.Embedder.BaseURL, cfg.Embedder.Model, cfg.Embedder.APIKey, defaults, log)

	dataDir := config.DataDir()
	appRepo := fs.NewApplicationRepository(dataDir)
	resumeRepo := fs.NewResumeRepository(dataDir)
	docLoader := loader.New()

	profileRepo, err := newSQLiteProfile(cfg)
	if err != nil {
		return nil, pipeline.ApplyConfig{}, err
	}

	augmentSvc := augment.New(profileRepo, profileRepo, embedderClient, llmClient, defaults, log)
	scorerSvc := scorer.New(defaults)
	clGen := coverletter.New(llmClient, defaults, log)
	fetcherSvc := fetcher.NewFallback(defaults, log)
	tailorSvc := tailor.New(llmClient, defaults, log)

	deps := pipeline.ApplyConfig{
		Fetcher:  fetcherSvc,
		LLM:      llmClient,
		Scorer:   scorerSvc,
		CLGen:    clGen,
		Resumes:  resumeRepo,
		Loader:   docLoader,
		AppRepo:  appRepo,
		Augment:  augmentSvc,
		Defaults: defaults,
		Tailor:   tailorSvc,
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
	urlVal := req.GetString("url", "")
	textVal := req.GetString("text", "")
	channelVal := req.GetString("channel", "COLD")
	accomplishmentsVal := req.GetString("accomplishments", "")

	if urlVal != "" && textVal != "" {
		return errorResult("exactly one of url or text is required")
	}
	if urlVal == "" && textVal == "" {
		return errorResult("exactly one of url or text is required")
	}

	channel, err := resolveChannel(channelVal)
	if err != nil {
		return errorResult(err.Error())
	}

	pres := mcppres.New()
	deps.Presenter = pres

	pl := pipeline.NewApplyPipeline(deps)

	isText := textVal != ""
	input := urlVal
	if isText {
		input = textVal
	}

	runErr := pl.Run(ctx, pipeline.ApplyRequest{
		URLOrText:           input,
		IsText:              isText,
		Channel:             channel,
		Config:              resolveConfig(cfg),
		AccomplishmentsPath: accomplishmentsVal,
	})
	if runErr != nil {
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
