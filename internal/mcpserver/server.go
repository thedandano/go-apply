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
			mcp.WithString("sections", mcp.Description("Optional JSON-encoded SectionMap (schema_version, contact, experience, …). When provided, validated and saved to disk alongside the resume.")),
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
			mcp.WithString("sections", mcp.Description("Optional JSON-encoded SectionMap (schema_version, contact, experience, …). When provided, validated and saved to disk alongside the resume.")),
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

	// ── Profile tools (compile, story creation) ───────────────────────────────

	srv.AddTool(
		mcp.NewTool("compile_profile",
			mcp.WithDescription("Read all skills and accomplishment files from the user's profile, tag each story with matching skills, and write a compiled profile artifact. Returns all stories (with skill tags and classification), orphaned skills (skills with no supporting story), and any LLM tagging failures. Call this after onboard_user, after adding a new skills file, or after the user reports adding a new accomplishment file."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleCompileProfile(ctx, &req), nil
		},
	)

	srv.AddTool(
		mcp.NewTool("create_story",
			mcp.WithDescription("Save a new accomplishment story in SBI (Situation-Behavior-Impact) format, classified by story type and job title, and trigger recompilation of the user's compiled profile. Use this when the user creates a story for an orphaned skill — either during onboarding or during a job application. The story is stored canonically in the profile; it is not JD-specific. After this call succeeds, the compiled profile is updated and any skills the story evidences are removed from remaining_orphans."),
			mcp.WithString("skill", mcp.Description("The primary skill this story supports. Must match a label present in the user's skills file."), mcp.Required()),
			mcp.WithString("story_type", mcp.Description("Classification: project, achievement, technical, leadership, process, or collaboration."), mcp.Required()),
			mcp.WithString("job_title", mcp.Description("The experience role this story belongs to. Should match a role in the user's career history. If new, set is_new_job=true and provide start/end dates."), mcp.Required()),
			mcp.WithBoolean("is_new_job", mcp.Description("Set to true if job_title is a new role not in the user's career history. Requires job_start_date and job_end_date.")),
			mcp.WithString("job_start_date", mcp.Description("Required when is_new_job=true. Format: YYYY-MM or YYYY.")),
			mcp.WithString("job_end_date", mcp.Description("Required when is_new_job=true. Format: YYYY-MM, YYYY, or 'present'.")),
			mcp.WithString("situation", mcp.Description("S — Situation: the context, team size, system state, or business problem the user faced."), mcp.Required()),
			mcp.WithString("behavior", mcp.Description("B — Behavior: what the user specifically did (their actions, not the team's)."), mcp.Required()),
			mcp.WithString("impact", mcp.Description("I — Impact: the measurable outcome. Include numbers, timeframe, and scale where possible."), mcp.Required()),
			mcp.WithString("jd_context", mcp.Description("Optional. The current job description text, used to seed the SBI prompts with relevant vocabulary. Not stored in the story.")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleCreateStory(ctx, &req), nil
		},
	)

	// ── Workflow tools (require onboarding) ───────────────────────────────────

	srv.AddTool(
		mcp.NewTool("load_jd",
			mcp.WithDescription("Start a job application workflow: fetch the job description by URL or accept raw text. Returns jd_text, session_id, and extraction_protocol — follow extraction_protocol exactly when building jd_json for submit_keywords."),
			mcp.WithString("jd_url", mcp.Description("URL of the job posting to fetch")),
			mcp.WithString("jd_raw_text", mcp.Description("Raw job description text (alternative to jd_url)")),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleLoadJD(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("submit_keywords",
			mcp.WithDescription("Submit extracted keywords to score resumes. Call after load_jd: follow the extraction_protocol in the load_jd response to extract keywords from jd_text, then provide them here as jd_json. Returns scores and a next_action directive."),
			mcp.WithString("session_id", mcp.Description("Session ID returned by load_jd"), mcp.Required()),
			mcp.WithString("jd_json", mcp.Description("JSON-encoded JDData with title, company, required, preferred, location, seniority, required_years"), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSubmitKeywords(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("submit_tailor_t1",
			mcp.WithDescription("Apply T1 tailoring: apply structured edits scoped to the Skills section and rescore. Call after submit_keywords when next_action is 'tailor_t1'. Provide edits as a JSON array of {section, op, value, category?} objects (max 5). Use op=replace or op=add; section must be 'skills'."),
			mcp.WithString("session_id", mcp.Description("Session ID from load_jd"), mcp.Required()),
			mcp.WithString("edits", mcp.Description(`JSON array of {"section":"skills","op":"replace|add","value":"...","category":"..."} objects. Max 5 entries. "category" is required when skills.kind="categorized" — use a key from sections.skills.categorized returned by submit_keywords. e.g. flat: [{"section":"skills","op":"add","value":"GCP"}] categorized: [{"section":"skills","category":"Cloud","op":"add","value":"GCP, Azure"}]`), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSubmitTailorT1(ctx, &req), nil
		}),
	)

	srv.AddTool(
		mcp.NewTool("submit_tailor_t2",
			mcp.WithDescription("Apply T2 tailoring: rewrite Experience bullets and rescore. Call after submit_tailor_t1. Provide edits as a JSON array of {section, op, target, value} objects; section must be 'experience'."),
			mcp.WithString("session_id", mcp.Description("Session ID from load_jd"), mcp.Required()),
			mcp.WithString("edits", mcp.Description(`JSON array of {"section":"experience","op":"replace|remove","target":"exp-<i>-b<j>","value":"..."} objects. e.g. [{"section":"experience","op":"replace","target":"exp-0-b2","value":"Led migration to Kubernetes"}]`), mcp.Required()),
		),
		requireOnboarded(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return HandleSubmitTailorT2(ctx, &req), nil
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
