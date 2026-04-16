// Package mcpserver provides the MCP stdio server for Claude Code integration.
package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/thedandano/go-apply/internal/config"
)

// NewServer creates and configures the MCP server with all tools and prompts registered.
func NewServer() *server.MCPServer {
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

	return srv
}

// Serve starts the MCP stdio server.
func Serve() error {
	return server.ServeStdio(NewServer())
}
