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

	for _, keyword := range []string{"get_score", "onboard_user", "jd_text", "best_score", "embedder"} {
		if !strings.Contains(text, keyword) {
			t.Errorf("workflow prompt missing keyword %q", keyword)
		}
	}
}
