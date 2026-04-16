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
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
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
				requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					cfg, deps, err := loadDeps()
					if err != nil {
						return errorResult(fmt.Sprintf("load config: %v", err)), nil
					}
					return HandleGetScoreWithConfig(ctx, &req, &deps, cfg), nil
				}),
			)

			srv.AddTool(
				mcp.NewTool("onboard_user",
					mcp.WithDescription("Store a resume, skills, and accomplishments into the profile database. All inputs are raw text extracted from the vector store."),
					mcp.WithString("resume_content", mcp.Description("Resume text (required when resume_label is provided)")),
					mcp.WithString("resume_label", mcp.Description("Short identifier for the resume, e.g. 'backend' (required when resume_content is provided)")),
					mcp.WithString("skills", mcp.Description("Skills reference text (optional)")),
					mcp.WithString("accomplishments", mcp.Description("Accomplishments text (optional)")),
				),
				func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					svc, cleanup, err := newOnboardSvc()
					if err != nil {
						return errorResult(fmt.Sprintf("setup: %v", err)), nil
					}
					defer cleanup()
					return HandleOnboardUser(ctx, &req, svc), nil
				},
			)

			srv.AddTool(
				mcp.NewTool("add_resume",
					mcp.WithDescription("Add or replace a single resume in the profile database."),
					mcp.WithString("resume_content", mcp.Description("Resume text"), mcp.Required()),
					mcp.WithString("resume_label", mcp.Description("Short identifier, e.g. 'backend'"), mcp.Required()),
				),
				func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					svc, cleanup, err := newOnboardSvc()
					if err != nil {
						return errorResult(fmt.Sprintf("setup: %v", err)), nil
					}
					defer cleanup()
					return HandleAddResume(ctx, &req, svc), nil
				},
			)

			srv.AddTool(
				mcp.NewTool("update_config",
					mcp.WithDescription("Set a go-apply config field by dot-notation key (e.g. embedder.model, embedding_dim, user_name). Orchestrator keys are not used in MCP mode."),
					mcp.WithString("key", mcp.Description("Dot-notation config key"), mcp.Required()),
					mcp.WithString("value", mcp.Description("New value for the key"), mcp.Required()),
				),
				func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					cfg, err := config.Load()
					if err != nil {
						return errorResult(fmt.Sprintf("load config: %v", err)), nil
					}
					return HandleUpdateConfig(ctx, &req, cfg), nil
				},
			)

			srv.AddTool(
				mcp.NewTool("get_config",
					mcp.WithDescription("Return all go-apply config fields. API keys are redacted."),
				),
				func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					cfg, err := config.Load()
					if err != nil {
						return errorResult(fmt.Sprintf("load config: %v", err)), nil
					}
					return HandleGetConfigWith(cfg), nil
				},
			)

			srv.AddPrompt(
				mcp.NewPrompt("job_application_workflow",
					mcp.WithPromptDescription("Orchestration guide: how Claude should use go-apply MCP tools to evaluate job fit and generate cover letters"),
				),
				HandleWorkflowPrompt,
			)

			return server.ServeStdio(srv)
		},
	}
}

// loadDeps loads configuration and wires all pipeline dependencies.
// Config is loaded fresh per invocation so changes take effect immediately.
// In MCP mode Claude is the orchestrator — LLM, CLGen, Augment, and Tailor are nil;
// Claude handles keyword extraction, cover letter generation, augmentation, and tailoring.
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
		LLM:      nil, // Claude handles keyword extraction
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
