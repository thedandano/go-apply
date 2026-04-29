package mcpserver_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
)

func newOnboardRequest(args map[string]interface{}) *mcp.CallToolRequest {
	req := &mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// TestHandleOnboardUserWith_ReturnsNeedsCompile verifies that HandleOnboardUserWith always
// returns needs_compile: true so the host knows to trigger recompilation.
func TestHandleOnboardUserWith_ReturnsNeedsCompile(t *testing.T) {
	dir := t.TempDir()
	svc := &stubOnboarder{
		result: model.OnboardResult{Stored: []string{"resume:backend"}},
	}

	req := newOnboardRequest(map[string]interface{}{
		"resume_content": "my resume",
		"resume_label":   "backend",
	})

	result := mcpserver.HandleOnboardUserWith(context.Background(), req, svc, dir)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v\nraw: %s", err, text)
	}

	if resp["needs_compile"] != true {
		t.Errorf("needs_compile = %v; want true", resp["needs_compile"])
	}
	if _, ok := resp["stored"]; !ok {
		t.Error("response missing 'stored' key")
	}
}
