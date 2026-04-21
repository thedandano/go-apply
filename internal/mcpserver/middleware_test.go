package mcpserver_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
)

// stubEmptyResumeRepo returns no resumes, simulating an unonboarded user.
type stubEmptyResumeRepo struct{}

func (s *stubEmptyResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return nil, nil
}

// ── CheckOnboarded unit tests ─────────────────────────────────────────────────

func TestCheckOnboarded_NoResumes_ReturnsError(t *testing.T) {
	err := mcpserver.CheckOnboarded(&stubEmptyResumeRepo{})
	if err == nil {
		t.Fatal("expected error when no resumes found, got nil")
	}
	if !strings.Contains(err.Error(), "no resumes found") {
		t.Errorf("error = %q, want to contain 'no resumes found'", err.Error())
	}
}

func TestCheckOnboarded_Onboarded_ReturnsNil(t *testing.T) {
	err := mcpserver.CheckOnboarded(&stubResumeRepo{}) // stubResumeRepo returns one resume
	if err != nil {
		t.Errorf("expected nil when onboarded, got: %v", err)
	}
}

// ── RequireOnboarded integration tests ───────────────────────────────────────

func TestRequireOnboarded_NotOnboarded_ReturnsErrorResult(t *testing.T) {
	req := callToolRequest("load_jd", map[string]any{"jd_url": "https://example.com/job"})

	called := false
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText(`{"status":"ok"}`), nil
	}

	handler := mcpserver.RequireOnboarded(&stubEmptyResumeRepo{}, inner)
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	if called {
		t.Error("inner handler must not be called when not onboarded")
	}

	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if resp["error"] == "" {
		t.Errorf("expected error key in response, got: %v", resp)
	}
}

func TestRequireOnboarded_Onboarded_DelegatesToInnerHandler(t *testing.T) {
	req := callToolRequest("load_jd", map[string]any{"jd_url": "https://example.com/job"})

	called := false
	inner := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText(`{"status":"ok"}`), nil
	}

	handler := mcpserver.RequireOnboarded(&stubResumeRepo{}, inner)
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	if !called {
		t.Error("inner handler must be called when onboarded")
	}

	text := extractText(t, result)
	if !strings.Contains(text, `"status":"ok"`) {
		t.Errorf("expected inner handler result, got: %s", text)
	}
}
