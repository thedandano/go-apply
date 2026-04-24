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
			mcp.WithString("sections", mcp.Description("Optional JSON-encoded SectionMap (schema_version, contact, experience, …). When provided, validated and persisted as a sidecar alongside the resume.")),
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
			mcp.WithString("sections", mcp.Description("Optional JSON-encoded SectionMap (schema_version, contact, experience, …). When provided, validated and persisted as a sidecar alongside the resume.")),
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
			mcp.WithDescription("Apply T1 tailoring: apply string replacements scoped to the Skills section and rescore. Call after submit_keywords when next_action is 'tailor_t1'. Provide skill_rewrites as a JSON array of {original, replacement} objects (max 5 per call). Prefer one-for-one swaps over pure appends to keep the section length stable."),
			mcp.WithString("session_id", mcp.Description("Session ID from load_jd"), mcp.Required()),
			mcp.WithString("skill_rewrites", mcp.Description("JSON array of {original, replacement} pairs scoped to Skills section, e.g. [{\"original\":\"AWS\",\"replacement\":\"AWS, GCP\"}]. Max 5 entries."), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSubmitTailorT1(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("submit_tailor_t2",
			mcp.WithDescription("Apply T2 tailoring: substitute Experience bullet points and rescore. Call after submit_tailor_t1. Provide bullet_rewrites as JSON array of {original, replacement} objects."),
			mcp.WithString("session_id", mcp.Description("Session ID from load_jd"), mcp.Required()),
			mcp.WithString("bullet_rewrites", mcp.Description("JSON array of {\"original\":\"...\",\"replacement\":\"...\"} objects"), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSubmitTailorT2(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("submit_edits",
			mcp.WithDescription("Apply structured edits (add/remove/replace) to the best resume's sections. Edits are applied in order; per-edit failures are reported in edits_rejected. Call after submit_keywords."),
			mcp.WithString("session_id", mcp.Description("Session ID from load_jd"), mcp.Required()),
			mcp.WithString("edits", mcp.Description(`JSON array of {"section":"skills|experience","op":"add|remove|replace","target":"exp-<i>-b<j>","value":"..."} objects. target required for replace/remove on experience bullets.`), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSubmitEdits(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("preview_ats_extraction",
			mcp.WithDescription("Return the constructed resume text as it would be seen by an ATS. Today an identity pass-through (raw text); the seam exists for future PDF render + extraction. Call after submit_keywords."),
			mcp.WithString("session_id", mcp.Description("Session ID from load_jd"), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandlePreviewATSExtraction(ctx, &req), nil
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
