package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// ── onboard stubs ─────────────────────────────────────────────────────────────

type stubOnboarder struct {
	result model.OnboardResult
	err    error
}

var _ port.Onboarder = (*stubOnboarder)(nil)

func (s *stubOnboarder) Run(_ context.Context, _ model.OnboardInput) (model.OnboardResult, error) {
	return s.result, s.err
}

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubJDFetcher struct{}

var _ port.JDFetcher = (*stubJDFetcher)(nil)

func (s *stubJDFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return "fake job description text", nil
}

type stubLLMClient struct{}

var _ port.LLMClient = (*stubLLMClient)(nil)

func (s *stubLLMClient) ChatComplete(_ context.Context, _ []model.ChatMessage, _ model.ChatOptions) (string, error) {
	return `{"title":"SWE","company":"Acme","required":["go"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`, nil
}

type stubScorer struct{}

var _ port.Scorer = (*stubScorer)(nil)

func (s *stubScorer) Score(_ *model.ScorerInput) (model.ScoreResult, error) {
	return model.ScoreResult{
		Breakdown: model.ScoreBreakdown{
			KeywordMatch:   0.9,
			ExperienceFit:  0.9,
			ImpactEvidence: 0.9,
			ATSFormat:      0.9,
			Readability:    0.9,
		},
	}, nil
}

type stubCoverLetterGen struct{}

var _ port.CoverLetterGenerator = (*stubCoverLetterGen)(nil)

func (s *stubCoverLetterGen) Generate(_ context.Context, _ *model.CoverLetterInput) (model.CoverLetterResult, error) {
	return model.CoverLetterResult{Text: "Cover letter.", Channel: model.ChannelCold}, nil
}

type stubResumeRepo struct{}

var _ port.ResumeRepository = (*stubResumeRepo)(nil)

func (s *stubResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return []model.ResumeFile{{Label: "main", Path: "/fake/main.txt"}}, nil
}

type stubDocumentLoader struct{}

var _ port.DocumentLoader = (*stubDocumentLoader)(nil)

func (s *stubDocumentLoader) Load(_ string) (string, error) {
	return "golang experience senior engineer 5 years", nil
}

func (s *stubDocumentLoader) Supports(_ string) bool { return true }

type stubApplicationRepository struct{}

var _ port.ApplicationRepository = (*stubApplicationRepository)(nil)

func (s *stubApplicationRepository) Get(_ string) (*model.ApplicationRecord, bool, error) {
	return nil, false, nil
}
func (s *stubApplicationRepository) Put(_ *model.ApplicationRecord) error    { return nil }
func (s *stubApplicationRepository) Update(_ *model.ApplicationRecord) error { return nil }
func (s *stubApplicationRepository) List() ([]*model.ApplicationRecord, error) {
	return nil, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// callToolRequest builds an mcp.CallToolRequest with the given arguments map.
// Arguments must be map[string]any (not json.RawMessage) so that GetString works correctly.
func callToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

// extractText pulls the first TextContent text from a CallToolResult.
func extractText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("CallToolResult has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is not TextContent: %T", result.Content[0])
	}
	return tc.Text
}

// ── HandleOnboardUser tests ────────────────────────────────────────────────────

func TestHandleOnboardUser_XORValidation_MissingLabel(t *testing.T) {
	svc := &stubOnboarder{}
	req := callToolRequest("onboard_user", map[string]any{
		"resume_content": "resume text",
		// resume_label missing
	})
	result := mcpserver.HandleOnboardUser(context.Background(), &req, svc)
	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected error key, got: %v", resp)
	}
}

func TestHandleOnboardUser_XORValidation_MissingContent(t *testing.T) {
	svc := &stubOnboarder{}
	req := callToolRequest("onboard_user", map[string]any{
		"resume_label": "backend",
		// resume_content missing
	})
	result := mcpserver.HandleOnboardUser(context.Background(), &req, svc)
	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected error key, got: %v", resp)
	}
}

func TestHandleOnboardUser_EmptyInput_ReturnsError(t *testing.T) {
	svc := &stubOnboarder{}
	req := callToolRequest("onboard_user", map[string]any{})
	result := mcpserver.HandleOnboardUser(context.Background(), &req, svc)
	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected error key, got: %v", resp)
	}
}

func TestHandleOnboardUser_HappyPath_ReturnsStored(t *testing.T) {
	svc := &stubOnboarder{
		result: model.OnboardResult{Stored: []string{"resume:backend"}},
	}
	req := callToolRequest("onboard_user", map[string]any{
		"resume_content": "resume text",
		"resume_label":   "backend",
	})
	result := mcpserver.HandleOnboardUser(context.Background(), &req, svc)
	text := extractText(t, result)
	var resp model.OnboardResult
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not valid OnboardResult JSON: %v — got: %s", err, text)
	}
	if len(resp.Stored) != 1 || resp.Stored[0] != "resume:backend" {
		t.Errorf("Stored = %v, want [resume:backend]", resp.Stored)
	}
}

func TestHandleOnboardUser_SkillsOnly_ReturnsError(t *testing.T) {
	svc := &stubOnboarder{}
	req := callToolRequest("onboard_user", map[string]any{
		"skills": "Go, Python",
	})
	result := mcpserver.HandleOnboardUser(context.Background(), &req, svc)
	text := extractText(t, result)
	if !strings.Contains(text, "resume is required") {
		t.Errorf("expected 'resume is required' error, got: %s", text)
	}
}

// ── HandleAddResume tests ─────────────────────────────────────────────────────

func TestHandleAddResume_MissingContent_ReturnsError(t *testing.T) {
	svc := &stubOnboarder{}
	req := callToolRequest("add_resume", map[string]any{
		"resume_label": "backend",
	})
	result := mcpserver.HandleAddResume(context.Background(), &req, svc)
	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected error key, got: %v", resp)
	}
}

func TestHandleAddResume_HappyPath(t *testing.T) {
	svc := &stubOnboarder{
		result: model.OnboardResult{Stored: []string{"resume:backend"}},
	}
	req := callToolRequest("add_resume", map[string]any{
		"resume_content": "resume text",
		"resume_label":   "backend",
	})
	result := mcpserver.HandleAddResume(context.Background(), &req, svc)
	text := extractText(t, result)
	var resp model.OnboardResult
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not valid OnboardResult JSON: %v — got: %s", err, text)
	}
}

// ── HandleUpdateConfig tests ──────────────────────────────────────────────────

func TestHandleUpdateConfig_MissingKey_ReturnsError(t *testing.T) {
	req := callToolRequest("update_config", map[string]any{
		"value": "claude-haiku-4-5",
	})
	cfg := &config.Config{}
	result := mcpserver.HandleUpdateConfig(context.Background(), &req, cfg)
	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected error key, got: %v", resp)
	}
}

func TestHandleUpdateConfig_UnknownKey_ReturnsError(t *testing.T) {
	req := callToolRequest("update_config", map[string]any{
		"key":   "nonexistent.field",
		"value": "value",
	})
	cfg := &config.Config{}
	result := mcpserver.HandleUpdateConfig(context.Background(), &req, cfg)
	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected error key, got: %v", resp)
	}
}

func TestHandleUpdateConfig_APIKey_ResponseRedacted(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	req := callToolRequest("update_config", map[string]any{
		"key":   "embedder.api_key",
		"value": "sk-super-secret",
	})
	cfg := &config.Config{}
	result := mcpserver.HandleUpdateConfig(context.Background(), &req, cfg)
	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if resp["value"] == "sk-super-secret" {
		t.Error("API key must be redacted in update_config response, got plaintext")
	}
	if resp["value"] != "***" {
		t.Errorf("update_config response value = %q, want ***", resp["value"])
	}
}

// ── handleGetConfigWith tests ─────────────────────────────────────────────────

func TestHandleGetConfigWith_RedactsAPIKeys(t *testing.T) {
	cfg := &config.Config{}
	cfg.Orchestrator.APIKey = "sk-super-secret"
	cfg.Embedder.APIKey = "another-key"

	result := mcpserver.HandleGetConfigWith(cfg)

	text := extractText(t, result)
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	// orchestrator keys are not exposed in MCP mode — only check embedder
	apiKey, ok := response["embedder.api_key"].(string)
	if !ok {
		t.Fatalf("embedder.api_key is not a string: %T", response["embedder.api_key"])
	}
	if apiKey != "***" {
		t.Errorf("embedder.api_key = %q, want ***", apiKey)
	}
}

func TestHandleGetConfigWith_EmptyAPIKey_NotRedacted(t *testing.T) {
	cfg := &config.Config{}
	// API keys left empty

	result := mcpserver.HandleGetConfigWith(cfg)

	text := extractText(t, result)
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	apiKey, ok := response["embedder.api_key"].(string)
	if !ok {
		t.Fatalf("embedder.api_key is not a string: %T", response["embedder.api_key"])
	}
	if apiKey == "***" {
		t.Error("empty API key should not be redacted")
	}
}

func TestHandleGetConfigWith_ExcludesOrchestratorKeys(t *testing.T) {
	result := mcpserver.HandleGetConfigWith(&config.Config{})

	text := extractText(t, result)
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"orchestrator.base_url", "orchestrator.model", "orchestrator.api_key"} {
		if _, found := response[key]; found {
			t.Errorf("get_config must not expose %q in MCP mode", key)
		}
	}
}

func TestHandleUpdateConfig_RejectsOrchestratorKey(t *testing.T) {
	req := callToolRequest("update_config", map[string]any{
		"key":   "orchestrator.model",
		"value": "gpt-4o",
	})
	result := mcpserver.HandleUpdateConfig(context.Background(), &req, &config.Config{})

	text := extractText(t, result)
	var payload map[string]string
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["error"] == "" {
		t.Error("expected error when setting orchestrator key in MCP mode")
	}
}

// ── HandleGetConfigWithProfile tests ──────────────────────────────────────────

func TestHandleGetConfigWithProfile_OnboardedTrue_WhenResumesExist(t *testing.T) {
	cfg := &config.Config{}
	cfg.Embedder.BaseURL = "http://localhost:11434/v1"
	cfg.Embedder.Model = "nomic-embed-text"

	// Create temp directory structure for test
	tmpDir := t.TempDir()
	inputsDir := filepath.Join(tmpDir, "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	// Create dummy resume files (resume extensions: .docx and .pdf, not .md/.txt which conflict with skills/accomplishments)
	if err := os.WriteFile(filepath.Join(inputsDir, "senior-swe.docx"), []byte("resume content"), 0o644); err != nil {
		t.Fatalf("write resume: %v", err)
	}
	if err := os.WriteFile(filepath.Join(inputsDir, "staff-swe.pdf"), []byte("resume content"), 0o644); err != nil {
		t.Fatalf("write resume: %v", err)
	}

	// Create skills file at dataDir root (source-scoped layout: skills go to dataDir, not inputs/).
	skillsPath := filepath.Join(tmpDir, "skills.md")
	if err := os.WriteFile(skillsPath, []byte("Go, Rust"), 0o644); err != nil {
		t.Fatalf("write skills: %v", err)
	}

	// Create accomplishments file at dataDir root using the accomplishments-N.md naming convention.
	accomplishmentsPath := filepath.Join(tmpDir, "accomplishments-0.md")
	if err := os.WriteFile(accomplishmentsPath, []byte("accomplished things"), 0o644); err != nil {
		t.Fatalf("write accomplishments: %v", err)
	}

	result := mcpserver.HandleGetConfigWithProfileAndFiles(cfg, tmpDir)
	text := extractText(t, result)

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("response is not JSON: %v — got: %s", err, text)
	}

	// Check profile object exists
	profileObj, ok := response["profile"]
	if !ok {
		t.Errorf("response missing 'profile' key: %v", response)
	}

	profile, ok := profileObj.(map[string]interface{})
	if !ok {
		t.Errorf("profile is not a map: %T", profileObj)
	}

	// Check onboarded is true
	onboarded, ok := profile["onboarded"].(bool)
	if !ok {
		t.Errorf("profile.onboarded is not a bool: %T", profile["onboarded"])
	}
	if !onboarded {
		t.Error("profile.onboarded = false, want true when resumes exist")
	}

	// Check resumes list — only the 2 actual resume files in inputs/ (skills/accomplishments
	// are now in the dataDir root, not inputs/, so they are not counted as resumes).
	resumesList, ok := profile["resumes"].([]interface{})
	if !ok {
		t.Errorf("profile.resumes is not a list: %T", profile["resumes"])
	}
	if len(resumesList) != 2 {
		t.Errorf("profile.resumes has %d items, want 2; got: %v", len(resumesList), resumesList)
	}

	// Check has_skills is true
	hasSkills, ok := profile["has_skills"].(bool)
	if !ok {
		t.Errorf("profile.has_skills is not a bool: %T", profile["has_skills"])
	}
	if !hasSkills {
		t.Error("profile.has_skills = false, want true when skills.md exists and is non-empty")
	}

	// Check has_accomplishments is true
	hasAccomplishments, ok := profile["has_accomplishments"].(bool)
	if !ok {
		t.Errorf("profile.has_accomplishments is not a bool: %T", profile["has_accomplishments"])
	}
	if !hasAccomplishments {
		t.Error("profile.has_accomplishments = false, want true when accomplishments-*.md exists and is non-empty")
	}
}

func TestHandleGetConfigWithProfile_OnboardedFalse_WhenNoResumes(t *testing.T) {
	cfg := &config.Config{}
	cfg.Embedder.BaseURL = "http://localhost:11434/v1"
	cfg.Embedder.Model = "nomic-embed-text"

	// Create temp directory with no resumes
	tmpDir := t.TempDir()
	inputsDir := filepath.Join(tmpDir, "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	result := mcpserver.HandleGetConfigWithProfileAndFiles(cfg, tmpDir)
	text := extractText(t, result)

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}

	profileObj, ok := response["profile"]
	if !ok {
		t.Errorf("response missing 'profile' key: %v", response)
	}

	profile, ok := profileObj.(map[string]interface{})
	if !ok {
		t.Errorf("profile is not a map: %T", profileObj)
	}

	onboarded, ok := profile["onboarded"].(bool)
	if !ok {
		t.Errorf("profile.onboarded is not a bool: %T", profile["onboarded"])
	}
	if onboarded {
		t.Error("profile.onboarded = true, want false when no resumes exist")
	}

	resumesList, ok := profile["resumes"].([]interface{})
	if !ok {
		t.Errorf("profile.resumes is not a list: %T", profile["resumes"])
	}
	if len(resumesList) != 0 {
		t.Errorf("profile.resumes has %d items, want 0", len(resumesList))
	}
}

func TestHandleGetConfigWithProfile_PreservesConfigFields(t *testing.T) {
	cfg := &config.Config{}
	cfg.Embedder.BaseURL = "http://localhost:11434/v1"
	cfg.Embedder.Model = "nomic-embed-text"
	cfg.Embedder.APIKey = "secret-key"

	tmpDir := t.TempDir()
	inputsDir := filepath.Join(tmpDir, "inputs")
	if err := os.MkdirAll(inputsDir, 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	result := mcpserver.HandleGetConfigWithProfileAndFiles(cfg, tmpDir)
	text := extractText(t, result)

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}

	// Check config fields still exist
	if _, ok := response["embedder.base_url"]; !ok {
		t.Error("response missing 'embedder.base_url' key")
	}
	if _, ok := response["embedder.model"]; !ok {
		t.Error("response missing 'embedder.model' key")
	}

	// API key should be redacted
	apiKey, ok := response["embedder.api_key"].(string)
	if !ok {
		t.Errorf("embedder.api_key is not a string: %T", response["embedder.api_key"])
	}
	if apiKey != "***" {
		t.Errorf("embedder.api_key = %q, want ***", apiKey)
	}
}
