package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

const workflowPromptText = `# go-apply MCP — Job Application Workflow

You are the orchestrator. go-apply's tools handle the mechanical work (fetch, embed, score).
You handle reasoning: extract keywords from JD text, interpret scores, write cover letters, give honest fit assessments.

## Tools

| Tool | Purpose | Key inputs |
|------|---------|------------|
| load_jd | Fetch JD + start session | jd_url OR jd_raw_text |
| submit_keywords | Score resumes against extracted JD | session_id (req), jd_json (req) |
| finalize | Persist record + close session | session_id (req), cover_letter (opt) |
| onboard_user | Store resume + skills + accomplishments | resume_content, resume_label, skills, accomplishments |
| add_resume | Add/replace a single resume | resume_content (req), resume_label (req) |
| get_config | Read current config (API keys redacted) | — |
| update_config | Set a config field by dot-notation key | key, value |

## Standard Multi-Turn Workflow

### Step 1 — Verify profile
Call get_config. Confirm embedder.base_url, embedder.model, and embedding_dim are set.
Do NOT ask the user for orchestrator config — it is irrelevant in MCP mode.
If the user has not onboarded yet, call onboard_user first.

### Step 2 — load_jd
Provide jd_url OR jd_raw_text (not both).
Returns: session_id + jd_text (raw job description text for you to reason over).

### Step 3 — Extract keywords yourself
From jd_text, extract:
- title, company, location, seniority, required_years
- required: 5–10 must-have skills
- preferred: 2–5 nice-to-have skills

Encode as JSON: {"title":"...","company":"...","required":[...],"preferred":[...],"location":"...","seniority":"senior","required_years":5}

### Step 4 — submit_keywords
Send session_id + jd_json.
Returns: scores, best_resume, best_score, next_action.

next_action values:
- "cover_letter" — score ≥ 0.70, strong fit
- "tailor_t1" — 0.40 ≤ score < 0.70, moderate fit (tailoring may help)
- "advise_skip" — score < 0.40, structural mismatch

### Step 5 — Interpret and act

| Score range | Action |
|-------------|--------|
| ≥ 0.70 | Draft cover letter, call finalize with cover_letter |
| 0.40–0.69 | Identify skill gaps, advise on tailoring |
| < 0.40 | Advise skip: "structural mismatch — tailoring cannot close this gap" |

### Step 6 — finalize
Send session_id and optional cover_letter text.
This persists the application record and closes the session.

## Config — only embedder + profile needed in MCP mode
- embedder.base_url, embedder.model, embedder.api_key: embedding service
- embedding_dim: output dimension of the embedding model (e.g. 2048)
- user_name, occupation, location, linkedin_url, years_of_experience: used in cover letters

Orchestrator config is NOT used in MCP mode — Claude is the orchestrator.

## Onboarding tailoring tiers
- resume only: stored. Tier 1 (keyword injection) and Tier 2 (bullet rewriting) unavailable — warn the user.
- + skills: Tier 1 available. Tier 2 unavailable — warn.
- + accomplishments: Tier 2 available. Tier 1 unavailable — warn.
- + skills + accomplishments: full tailoring pipeline active.`

// HandleWorkflowPrompt is the exported handler for the "job_application_workflow" MCP prompt.
func HandleWorkflowPrompt(_ context.Context, _ mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: "Orchestration guide: how Claude should use go-apply MCP tools to evaluate job fit and generate cover letters",
		Messages: []mcp.PromptMessage{
			mcp.NewPromptMessage(mcp.RoleUser, mcp.NewTextContent(workflowPromptText)),
		},
	}, nil
}
