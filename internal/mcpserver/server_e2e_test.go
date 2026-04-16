//go:build integration

package mcpserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/mcpserver"
)

// newEmbedderStub returns a stub HTTP server that serves a fixed 3-element
// embedding vector at /embeddings. Used by onboard_user (and indirectly
// get_score after onboarding) to avoid a real LLM dependency.
func newEmbedderStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/embeddings" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"embedding": []float32{0.1, 0.2, 0.3}}},
			})
			return
		}
		http.NotFound(w, r)
	}))
}

// setupTestEnv redirects all config and data I/O to isolated temp dirs by
// setting XDG_CONFIG_HOME and XDG_DATA_HOME, then writes a minimal
// config.yaml with the given embedder base URL (no /v1 suffix — the LLM
// client appends /embeddings directly to whatever base_url is set).
// It also pre-creates the data subdirectories (go-apply/ and inputs/) so that
// both SQLite and the resume repository can operate without hitting missing-dir errors.
func setupTestEnv(t *testing.T, embedderURL string) {
	t.Helper()
	tmp := t.TempDir()
	cfgBase := filepath.Join(tmp, "config")
	dataBase := filepath.Join(tmp, "data")
	t.Setenv("XDG_CONFIG_HOME", cfgBase)
	t.Setenv("XDG_DATA_HOME", dataBase)

	cfgDir := filepath.Join(cfgBase, "go-apply")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdirall cfgDir: %v", err)
	}
	cfgContent := "embedder:\n  base_url: " + embedderURL + "\n  model: test-model\nembedding_dim: 3\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfgContent), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	// Pre-create data subdirectories so SQLite and the resume repository don't
	// fail on missing parent directories.
	for _, sub := range []string{"go-apply", filepath.Join("go-apply", "inputs")} {
		if err := os.MkdirAll(filepath.Join(dataBase, sub), 0o700); err != nil {
			t.Fatalf("mkdirall %s: %v", sub, err)
		}
	}
}

// newMCPClient creates an in-process MCP client backed by mcpserver.NewServer(),
// starts it, and completes the MCP Initialize handshake. The client is closed
// automatically via t.Cleanup.
func newMCPClient(t *testing.T) *client.Client {
	t.Helper()
	cl, err := client.NewInProcessClient(mcpserver.NewServer())
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	ctx := context.Background()
	if err := cl.Start(ctx); err != nil {
		t.Fatalf("client Start: %v", err)
	}
	if _, err := cl.Initialize(ctx, mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "test-client", Version: "0.0.1"},
		},
	}); err != nil {
		t.Fatalf("client Initialize: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close() })
	return cl
}

// callTool dispatches a CallTool request through the MCP server and returns
// the first text content string from the result.
func callTool(t *testing.T, cl *client.Client, name string, args map[string]any) string {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	result, err := cl.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatalf("CallTool %s: no text content in result", name)
	return ""
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestServerDispatch_ToolsRegistered verifies that all five tools are
// discoverable through the live MCP server.
func TestServerDispatch_ToolsRegistered(t *testing.T) {
	stub := newEmbedderStub(t)
	defer stub.Close()
	setupTestEnv(t, stub.URL)
	cl := newMCPClient(t)

	result, err := cl.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	want := map[string]bool{
		"get_score":     false,
		"onboard_user":  false,
		"add_resume":    false,
		"update_config": false,
		"get_config":    false,
	}
	for _, tool := range result.Tools {
		want[tool.Name] = true
	}
	for name, found := range want {
		if !found {
			t.Errorf("tool %q not registered", name)
		}
	}
}

// TestServerDispatch_PromptsRegistered verifies that the job_application_workflow
// prompt is registered and returns non-empty content.
func TestServerDispatch_PromptsRegistered(t *testing.T) {
	stub := newEmbedderStub(t)
	defer stub.Close()
	setupTestEnv(t, stub.URL)
	cl := newMCPClient(t)

	const wantPrompt = "job_application_workflow"

	listResult, err := cl.ListPrompts(context.Background(), mcp.ListPromptsRequest{})
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	found := false
	for _, p := range listResult.Prompts {
		if p.Name == wantPrompt {
			found = true
		}
	}
	if !found {
		t.Fatalf("prompt %q not listed", wantPrompt)
	}

	getResult, err := cl.GetPrompt(context.Background(), mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{Name: wantPrompt},
	})
	if err != nil {
		t.Fatalf("GetPrompt %s: %v", wantPrompt, err)
	}
	if len(getResult.Messages) == 0 {
		t.Errorf("prompt %q returned no messages", wantPrompt)
	}
}

// TestServerDispatch_GetScore_BlockedUntilOnboarded verifies that the
// requireOnboarded middleware rejects get_score calls when no resumes exist.
func TestServerDispatch_GetScore_BlockedUntilOnboarded(t *testing.T) {
	stub := newEmbedderStub(t)
	defer stub.Close()
	setupTestEnv(t, stub.URL)
	cl := newMCPClient(t)

	raw := callTool(t, cl, "get_score", map[string]any{
		"text":    "Senior Go Engineer. Requires go, kubernetes.",
		"channel": "COLD",
	})

	var resp map[string]string
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v — raw: %s", err, raw)
	}
	if !strings.Contains(resp["error"], "no resumes found") {
		t.Errorf("expected 'no resumes found' in error, got: %s", resp["error"])
	}
}

// TestServerDispatch_GetConfig_ReturnsMCPKeys verifies that get_config returns
// all MCP-relevant keys and excludes orchestrator keys (Claude is the orchestrator
// in MCP mode).
func TestServerDispatch_GetConfig_ReturnsMCPKeys(t *testing.T) {
	stub := newEmbedderStub(t)
	defer stub.Close()
	setupTestEnv(t, stub.URL)
	cl := newMCPClient(t)

	raw := callTool(t, cl, "get_config", nil)

	var fields map[string]string
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatalf("unmarshal: %v — raw: %s", err, raw)
	}
	for _, key := range []string{"embedder.base_url", "embedder.model", "embedding_dim", "user_name"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("key %q missing from get_config response", key)
		}
	}
	if _, ok := fields["orchestrator.base_url"]; ok {
		t.Error("orchestrator.base_url must not appear in MCP mode config")
	}
}

// TestServerDispatch_UpdateConfig_PersistsField verifies that update_config
// writes the new value and that a subsequent get_config call returns it.
func TestServerDispatch_UpdateConfig_PersistsField(t *testing.T) {
	stub := newEmbedderStub(t)
	defer stub.Close()
	setupTestEnv(t, stub.URL)
	cl := newMCPClient(t)

	updateRaw := callTool(t, cl, "update_config", map[string]any{
		"key":   "user_name",
		"value": "Test User",
	})
	var updateResp map[string]string
	if err := json.Unmarshal([]byte(updateRaw), &updateResp); err != nil {
		t.Fatalf("unmarshal update response: %v — raw: %s", err, updateRaw)
	}
	if updateResp["updated"] != "user_name" {
		t.Errorf("update_config updated = %q, want user_name", updateResp["updated"])
	}

	configRaw := callTool(t, cl, "get_config", nil)
	var fields map[string]string
	if err := json.Unmarshal([]byte(configRaw), &fields); err != nil {
		t.Fatalf("unmarshal get_config: %v — raw: %s", err, configRaw)
	}
	if fields["user_name"] != "Test User" {
		t.Errorf("get_config user_name = %q, want Test User", fields["user_name"])
	}
}

// TestServerDispatch_OnboardThenScore verifies the full onboard → score flow
// through the MCP server: onboard_user stores a resume, then get_score passes
// the requireOnboarded middleware and returns a PipelineResult (identified by
// the presence of a "status" field).
func TestServerDispatch_OnboardThenScore(t *testing.T) {
	stub := newEmbedderStub(t)
	defer stub.Close()
	setupTestEnv(t, stub.URL)
	cl := newMCPClient(t)

	// Step 1: onboard a resume.
	onboardRaw := callTool(t, cl, "onboard_user", map[string]any{
		"resume_content": "golang kubernetes senior engineer five years experience",
		"resume_label":   "main",
	})
	var onboardResp map[string]any
	if err := json.Unmarshal([]byte(onboardRaw), &onboardResp); err != nil {
		t.Fatalf("unmarshal onboard: %v — raw: %s", err, onboardRaw)
	}
	if errMsg, hasErr := onboardResp["error"]; hasErr {
		t.Fatalf("onboard_user failed: %v", errMsg)
	}

	// Step 2: get_score should now pass the middleware and reach the pipeline.
	// In MCP mode LLM is nil, so the pipeline returns a structured error result
	// (status "error") rather than a plain-text error — the important thing is
	// that the response has a "status" field, proving middleware was not blocking.
	scoreRaw := callTool(t, cl, "get_score", map[string]any{
		"text":    "Senior Go Engineer at Acme. Requires: go, kubernetes.",
		"channel": "COLD",
	})
	var scoreResp map[string]any
	if err := json.Unmarshal([]byte(scoreRaw), &scoreResp); err != nil {
		t.Fatalf("unmarshal score: %v — raw: %s", err, scoreRaw)
	}
	if _, hasStatus := scoreResp["status"]; !hasStatus {
		t.Errorf("get_score response missing 'status' field — middleware may still be blocking: %s", scoreRaw)
	}
}
