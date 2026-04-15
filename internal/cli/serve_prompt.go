package cli

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
| get_score | Fetch JD → embed → score resumes | url OR text, channel, accomplishments |
| onboard_user | Store resume + skills + accomplishments | resume_content, resume_label, skills, accomplishments |
| add_resume | Add/replace a single resume | resume_content (req), resume_label (req) |
| get_config | Read current config (API keys redacted) | — |
| update_config | Set a config field by dot-notation key | key, value |

## Standard Workflow

### Step 1 — Verify profile
Call get_config. Confirm embedder.base_url, embedder.model, and embedding_dim are set.
If the user has not onboarded yet, call onboard_user first.

### Step 2 — Call get_score
Provide url OR text (not both). Set channel: COLD (default) | REFERRAL | RECRUITER.

### Step 3 — Interpret the result

| Field | Meaning |
|-------|---------|
| status | "success", "degraded", or "error" |
| jd_text | Raw job description — YOUR input for keyword extraction and reasoning |
| best_score | 0.0–1.0 keyword and heuristic fit score |
| best_resume | Which stored resume matched best |
| keywords.required/preferred | May be empty in MCP mode — extract from jd_text yourself |
| warnings | Degraded step notices |

### Step 4 — Your orchestration responsibilities
1. Extract 5–10 key requirements from jd_text
2. Interpret best_score: >= 0.70 strong fit | 0.40–0.69 moderate | < 0.40 structural mismatch
3. Identify top 3–5 skill matches and 2–3 honest gaps
4. Generate a cover letter if best_score >= 0.70 OR channel is REFERRAL/RECRUITER
5. Give a clear recommendation: apply / skip / apply with caveats

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
