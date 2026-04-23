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
// Tool ordering: setup tools first (always available), then workflow tools (require onboarding).
func NewServer() *server.MCPServer {
	srv := server.NewMCPServer("go-apply", "0.1.0")

	// ── Setup tools (no onboarding required) ──────────────────────────────────

	srv.AddTool(
		mcp.NewTool("onboard_user",
			mcp.WithDescription("Store a resume, skills, and accomplishments into the profile database. All inputs are raw text extracted from the vector store."),
			mcp.WithString("resume_content", mcp.Description("Resume text (required when resume_label is provided)")),
			mcp.WithString("resume_label", mcp.Description("Short identifier for the resume, e.g. 'backend' (required when resume_content is provided)")),
			mcp.WithString("skills", mcp.Description("Skills reference text (optional)")),
			mcp.WithString("accomplishments", mcp.Description("Accomplishments text (optional)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleOnboardUser(ctx, &req, newOnboardSvc()), nil
		},
	)

	srv.AddTool(
		mcp.NewTool("add_resume",
			mcp.WithDescription("Add or replace a single resume in the profile database."),
			mcp.WithString("resume_content", mcp.Description("Resume text"), mcp.Required()),
			mcp.WithString("resume_label", mcp.Description("Short identifier, e.g. 'backend'"), mcp.Required()),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleAddResume(ctx, &req, newOnboardSvc()), nil
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

	srv.AddTool(
		mcp.NewTool("update_config",
			mcp.WithDescription("Set a go-apply config field by dot-notation key (e.g. user_name, log_level, verbose). Orchestrator keys are not used in MCP mode."),
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

	// ── Workflow tools (require onboarding) ───────────────────────────────────

	srv.AddTool(
		mcp.NewTool("load_jd",
			mcp.WithDescription("Start a job application workflow: fetch the job description by URL or accept raw text. Returns jd_text for keyword extraction and a session_id to use in subsequent calls."),
			mcp.WithString("jd_url", mcp.Description("URL of the job posting to fetch")),
			mcp.WithString("jd_raw_text", mcp.Description("Raw job description text (alternative to jd_url)")),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleLoadJD(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("submit_keywords",
			mcp.WithDescription("Submit extracted keywords to score resumes. Call after load_jd: extract keywords from jd_text yourself, then provide them here as jd_json. Returns scores and a next_action directive."),
			mcp.WithString("session_id", mcp.Description("Session ID returned by load_jd"), mcp.Required()),
			mcp.WithString("jd_json", mcp.Description("JSON-encoded JDData with title, company, required, preferred, location, seniority, required_years"), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSubmitKeywords(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("submit_tailor_t1",
			mcp.WithDescription("Apply T1 tailoring: inject skill keywords into the resume's Skills section and rescore. Call after submit_keywords when next_action is 'tailor_t1'. Provide skill_adds as a JSON array of strings. Response data includes tailored_text (the full rewritten resume), added_keywords, skills_section_found, previous_score, and new_score."),
			mcp.WithString("session_id", mcp.Description("Session ID from load_jd"), mcp.Required()),
			mcp.WithString("skill_adds", mcp.Description("JSON array of skill strings to inject, e.g. [\"Kubernetes\",\"Terraform\"]"), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSubmitTailorT1(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("submit_tailor_t2",
			mcp.WithDescription("Apply T2 tailoring: substitute Experience bullet points and rescore. Call after submit_tailor_t1. Provide bullet_rewrites as JSON array of {original, replacement} objects. Response data includes tailored_text (the full rewritten resume), substitutions_made, previous_score, and new_score."),
			mcp.WithString("session_id", mcp.Description("Session ID from load_jd"), mcp.Required()),
			mcp.WithString("bullet_rewrites", mcp.Description("JSON array of {\"original\":\"...\",\"replacement\":\"...\"} objects"), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSubmitTailorT2(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("tailor_begin",
			mcp.WithDescription("Open a new LLM tailor session. Returns a session_id and prompt_bundle for Claude to use. Claude must call submit_tailored_resume when done."),
			mcp.WithString("resume_text", mcp.Description("Full resume text to tailor"), mcp.Required()),
			mcp.WithString("accomplishments_text", mcp.Description("Accomplishments document text (optional)")),
			mcp.WithString("jd", mcp.Description("Job description JSON (optional)")),
			mcp.WithString("score_before", mcp.Description("Score result JSON before tailoring (optional)")),
			mcp.WithString("options", mcp.Description("Tailor options JSON (optional)")),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleTailorBegin(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("tailor_submit",
			mcp.WithDescription("Submit the tailored resume produced by the LLM. Delivers the result back to the pipeline that is awaiting it."),
			mcp.WithString("session_id", mcp.Description("Session ID from tailor_begin"), mcp.Required()),
			mcp.WithString("tailored_text", mcp.Description("The full tailored resume text"), mcp.Required()),
			mcp.WithString("changelog", mcp.Description("JSON array of ChangelogEntry objects describing changes made (optional)")),
			mcp.WithString("tier_1_text", mcp.Description("Tier-1-only tailored text (optional)")),
			mcp.WithString("raw_changelog", mcp.Description("Raw markdown changelog (optional)")),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleTailorSubmit(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("finalize",
			mcp.WithDescription("Persist the application record and close the session. Optionally include a cover letter. Call after submit_keywords (and optionally submit_tailor_t1/t2)."),
			mcp.WithString("session_id", mcp.Description("Session ID from load_jd"), mcp.Required()),
			mcp.WithString("cover_letter", mcp.Description("Cover letter text to store with the record (optional)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleFinalize(ctx, &req), nil
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
