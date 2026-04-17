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
// embedding vector at /embeddings. Used by onboard_user to avoid a real
// embedding model dependency.
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
// setting XDG_CONFIG_HOME and XDG_DATA_HOME, then writes a config.yaml with
// the given embedder base URL (no /v1 suffix — the LLM client appends
// /embeddings directly to whatever base_url is set).
// It also pre-creates the data subdirectories (go-apply/ and inputs/) so that
// both SQLite and the resume repository can operate without missing-dir errors.
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

// jdRawText is a realistic job description used across tests. In the MCP flow
// this is the raw text that the MCP host (Claude) would provide to load_jd.
const jdRawText = "We are looking for a Senior Go Engineer to join our platform team at Acme Corp. " +
	"You will design and build microservices on Kubernetes, mentor junior engineers, and drive best practices across the org. " +
	"Requirements: 5+ years of Go, strong Kubernetes experience, familiarity with Docker and Terraform. " +
	"Nice to have: experience with gRPC and observability tooling."

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
		"load_jd":          false,
		"submit_keywords":  false,
		"submit_tailor_t1": false,
		"submit_tailor_t2": false,
		"finalize":         false,
		"onboard_user":     false,
		"add_resume":       false,
		"update_config":    false,
		"get_config":       false,
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

// TestServerDispatch_LoadJD_BlockedUntilOnboarded verifies that the
// requireOnboarded middleware rejects load_jd calls when no resumes exist.
func TestServerDispatch_LoadJD_BlockedUntilOnboarded(t *testing.T) {
	stub := newEmbedderStub(t)
	defer stub.Close()
	setupTestEnv(t, stub.URL)
	cl := newMCPClient(t)

	raw := callTool(t, cl, "load_jd", map[string]any{
		"jd_raw_text": jdRawText,
	})

	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v — raw: %s", err, raw)
	}
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "no resumes found") {
		t.Errorf("expected 'no resumes found' in error, got: %s", errMsg)
	}
	// Middleware-level errors must not contain scoring fields.
	if _, hasScore := resp["best_score"]; hasScore {
		t.Error("middleware-blocked response should not contain best_score")
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

	var fields map[string]any
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
	var fields map[string]any
	if err := json.Unmarshal([]byte(configRaw), &fields); err != nil {
		t.Fatalf("unmarshal get_config: %v — raw: %s", err, configRaw)
	}
	if fields["user_name"] != "Test User" {
		t.Errorf("get_config user_name = %q, want Test User", fields["user_name"])
	}
}

// TestServerDispatch_OnboardThenScore verifies the full onboard → load_jd →
// submit_keywords flow through the MCP server: onboard_user stores a resume,
// load_jd starts a session, and submit_keywords (with host-extracted keywords)
// scores the resume and returns an envelope with a positive best_score.
func TestServerDispatch_OnboardThenScore(t *testing.T) {
	embedder := newEmbedderStub(t)
	defer embedder.Close()
	setupTestEnv(t, embedder.URL)
	cl := newMCPClient(t)

	// Step 1: onboard resume.
	onboardRaw := callTool(t, cl, "onboard_user", map[string]any{
		"resume_content": "Senior Go Engineer with 5 years of experience building distributed systems in Go. Proficient in Kubernetes, Docker, Terraform, and gRPC.",
		"resume_label":   "main",
	})
	var onboardResp map[string]any
	if err := json.Unmarshal([]byte(onboardRaw), &onboardResp); err != nil {
		t.Fatalf("unmarshal onboard: %v — raw: %s", err, onboardRaw)
	}
	if errMsg, hasErr := onboardResp["error"]; hasErr {
		t.Fatalf("onboard_user failed: %v", errMsg)
	}

	// Step 2: load_jd — start a session with the raw JD text.
	loadRaw := callTool(t, cl, "load_jd", map[string]any{
		"jd_raw_text": jdRawText,
	})
	var loadResp map[string]any
	if err := json.Unmarshal([]byte(loadRaw), &loadResp); err != nil {
		t.Fatalf("unmarshal load_jd: %v — raw: %s", err, loadRaw)
	}
	if loadResp["status"] != "ok" {
		t.Fatalf("load_jd status = %v, want ok — full: %s", loadResp["status"], loadRaw)
	}
	sessionID, _ := loadResp["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	// Step 3: submit_keywords — the test plays the MCP host role by providing
	// pre-extracted keywords (matching the stub JD content).
	const jdJSON = `{"title":"Senior Go Engineer","company":"Acme Corp","required":["go","kubernetes"],"preferred":["docker","terraform"],"location":"Remote","seniority":"senior","required_years":5}`
	scoreRaw := callTool(t, cl, "submit_keywords", map[string]any{
		"session_id": sessionID,
		"jd_json":    jdJSON,
	})
	var scoreResp map[string]any
	if err := json.Unmarshal([]byte(scoreRaw), &scoreResp); err != nil {
		t.Fatalf("unmarshal submit_keywords: %v — raw: %s", err, scoreRaw)
	}

	if scoreResp["status"] != "ok" {
		t.Errorf("status = %v, want ok — full response: %s", scoreResp["status"], scoreRaw)
	}
	data, _ := scoreResp["data"].(map[string]any)
	if data == nil {
		t.Fatalf("expected data in submit_keywords response")
	}
	bestScore, _ := data["best_score"].(float64)
	if bestScore <= 0 {
		t.Errorf("best_score = %v, want > 0", data["best_score"])
	}
	if data["best_resume"] != "main" {
		t.Errorf("best_resume = %v, want main", data["best_resume"])
	}
}
