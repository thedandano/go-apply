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

	for _, keyword := range []string{"load_jd", "submit_keywords", "finalize", "onboard_user", "jd_text", "best_score", "embedder"} {
		if !strings.Contains(text, keyword) {
			t.Errorf("workflow prompt missing keyword %q", keyword)
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
