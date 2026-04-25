package mcpserver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/mcpserver"
)

func TestWorkflowPromptHandler_ReturnsUserMessage(t *testing.T) {
	result, err := mcpserver.HandleWorkflowPrompt(context.Background(), mcp.GetPromptRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message, got none")
	}
	if result.Messages[0].Role != mcp.RoleUser {
		t.Errorf("expected role %q, got %q", mcp.RoleUser, result.Messages[0].Role)
	}
}

func TestWorkflowPromptHandler_ContainsKeyWorkflowConcepts(t *testing.T) {
	result, _ := mcpserver.HandleWorkflowPrompt(context.Background(), mcp.GetPromptRequest{})
	text := result.Messages[0].Content.(mcp.TextContent).Text

	for _, keyword := range []string{"load_jd", "submit_keywords", "finalize", "onboard_user", "jd_text", "best_score"} {
		if !strings.Contains(text, keyword) {
			t.Errorf("workflow prompt missing keyword %q", keyword)
		}
	}
}

func TestWorkflowPromptHandler_ContainsResponseFormatDirectives(t *testing.T) {
	result, _ := mcpserver.HandleWorkflowPrompt(context.Background(), mcp.GetPromptRequest{})
	text := result.Messages[0].Content.(mcp.TextContent).Text

	for _, want := range []string{"| Keyword match", "Honest take", "My take"} {
		if !strings.Contains(text, want) {
			t.Errorf("workflow prompt missing response format directive %q", want)
		}
	}
}

func TestWorkflowPromptHandler_ContainsTailorTools(t *testing.T) {
	result, _ := mcpserver.HandleWorkflowPrompt(context.Background(), mcp.GetPromptRequest{})
	text := result.Messages[0].Content.(mcp.TextContent).Text

	for _, want := range []string{"submit_tailor_t1", "submit_tailor_t2"} {
		if !strings.Contains(text, want) {
			t.Errorf("workflow prompt missing tailor tool %q", want)
		}
	}
}

// SC-004: category field must appear in both the tool schema and the workflow prompt.
func TestWorkflowPromptHandler_ContainsCategoryGuidance(t *testing.T) {
	result, _ := mcpserver.HandleWorkflowPrompt(context.Background(), mcp.GetPromptRequest{})
	text := result.Messages[0].Content.(mcp.TextContent).Text

	for _, want := range []string{"category", "categorized", "skills_section.kind"} {
		if !strings.Contains(text, want) {
			t.Errorf("workflow prompt missing category guidance %q (SC-004)", want)
		}
	}
}

// T013a: FR-006 testability requirement — prompt must contain "prefer one-for-one" guidance.
func TestWorkflowPromptHandler_ContainsSkillRewriteGuidance(t *testing.T) {
	result, _ := mcpserver.HandleWorkflowPrompt(context.Background(), mcp.GetPromptRequest{})
	text := result.Messages[0].Content.(mcp.TextContent).Text

	if !strings.Contains(text, "prefer one-for-one") {
		t.Error("workflow prompt missing 'prefer one-for-one' tailoring guidance (FR-006)")
	}
	if !strings.Contains(text, "edits") {
		t.Error("workflow prompt must reference edits parameter")
	}
}

func TestWorkflowPromptHandler_ContainsTailoringTools(t *testing.T) {
	result, _ := mcpserver.HandleWorkflowPrompt(context.Background(), mcp.GetPromptRequest{})
	text := result.Messages[0].Content.(mcp.TextContent).Text

	for _, want := range []string{"submit_tailor_t1", "submit_tailor_t2", "preview_ats_extraction"} {
		if !strings.Contains(text, want) {
			t.Errorf("workflow prompt missing tool %q", want)
		}
	}
	if strings.Contains(text, "submit_edits") {
		t.Error("workflow prompt must not reference removed tool submit_edits")
	}
}

func TestWorkflowPromptHandler_SubmitKeywordsDescribesSectionsFields(t *testing.T) {
	result, _ := mcpserver.HandleWorkflowPrompt(context.Background(), mcp.GetPromptRequest{})
	text := result.Messages[0].Content.(mcp.TextContent).Text

	for _, want := range []string{"skills_section_found", "sections"} {
		if !strings.Contains(text, want) {
			t.Errorf("workflow prompt submit_keywords section missing response field %q", want)
		}
	}
}

func TestWorkflowPromptHandler_ScoreThresholdsAre0To100(t *testing.T) {
	result, _ := mcpserver.HandleWorkflowPrompt(context.Background(), mcp.GetPromptRequest{})
	text := result.Messages[0].Content.(mcp.TextContent).Text

	for _, want := range []string{"70", "/100"} {
		if !strings.Contains(text, want) {
			t.Errorf("workflow prompt missing score scale indicator %q", want)
		}
	}
	// Must not use 0–1 fractional thresholds.
	for _, bad := range []string{"≥ 0.70", "≥ 0.40", "< 0.40", "0.40–0.69"} {
		if strings.Contains(text, bad) {
			t.Errorf("workflow prompt contains fractional threshold %q — should use 0–100 scale", bad)
		}
	}
}
