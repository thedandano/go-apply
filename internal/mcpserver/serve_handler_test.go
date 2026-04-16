package mcpserver_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
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

type stubAugmenter struct{}

var _ port.Augmenter = (*stubAugmenter)(nil)

func (s *stubAugmenter) AugmentResumeText(_ context.Context, input model.AugmentInput) (string, *model.ReferenceData, error) {
	return input.ResumeText, input.RefData, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// stubApplyConfig returns an ApplyConfig with all stubs — no filesystem/SQLite.
func stubApplyConfig() pipeline.ApplyConfig {
	return pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       &stubLLMClient{},
		Scorer:    &stubScorer{},
		CLGen:     &stubCoverLetterGen{},
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubApplicationRepository{},
		Augment:   &stubAugmenter{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil, // overridden by handler internals
	}
}

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

// ── tests ─────────────────────────────────────────────────────────────────────

func TestGetScore_BothURLAndText_ReturnsError(t *testing.T) {
	cfg := stubApplyConfig()
	req := callToolRequest("get_score", map[string]any{
		"url":  "https://example.com/job",
		"text": "raw jd text",
	})

	result := mcpserver.HandleGetScore(context.Background(), &req, &cfg)

	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v — got: %s", err, text)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected error key in response, got: %v", resp)
	}
}

func TestGetScore_URLOnly_ReturnsResult(t *testing.T) {
	cfg := stubApplyConfig()
	req := callToolRequest("get_score", map[string]any{
		"url": "https://example.com/job",
	})

	result := mcpserver.HandleGetScore(context.Background(), &req, &cfg)

	text := extractText(t, result)
	var pipelineResult model.PipelineResult
	if err := json.Unmarshal([]byte(text), &pipelineResult); err != nil {
		t.Fatalf("response is not valid PipelineResult JSON: %v — got: %s", err, text)
	}
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
	var fields map[string]string
	if err := json.Unmarshal([]byte(text), &fields); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	// orchestrator keys are not exposed in MCP mode — only check embedder
	if fields["embedder.api_key"] != "***" {
		t.Errorf("embedder.api_key = %q, want ***", fields["embedder.api_key"])
	}
}

func TestHandleGetConfigWith_EmptyAPIKey_NotRedacted(t *testing.T) {
	cfg := &config.Config{}
	// API keys left empty

	result := mcpserver.HandleGetConfigWith(cfg)

	text := extractText(t, result)
	var fields map[string]string
	if err := json.Unmarshal([]byte(text), &fields); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if fields["embedder.api_key"] == "***" {
		t.Error("empty API key should not be redacted")
	}
}

func TestHandleGetConfigWith_ExcludesOrchestratorKeys(t *testing.T) {
	result := mcpserver.HandleGetConfigWith(&config.Config{})

	text := extractText(t, result)
	var fields map[string]string
	if err := json.Unmarshal([]byte(text), &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"orchestrator.base_url", "orchestrator.model", "orchestrator.api_key"} {
		if _, found := fields[key]; found {
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

// TestHandleGetScore_NilLLM_ErrorsWithActionableMessage verifies that when
// keyword extraction cannot produce a JD (nil LLM → empty JD), the handler
// returns an error result with an actionable message directing the user to
// supply the job description text directly.
func TestHandleGetScore_NilLLM_ErrorsWithActionableMessage(t *testing.T) {
	defaults, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	deps := pipeline.ApplyConfig{
		Fetcher:  &stubJDFetcher{},
		LLM:      nil,
		Scorer:   &stubScorer{},
		CLGen:    nil,
		Resumes:  &stubResumeRepo{},
		Loader:   &stubDocumentLoader{},
		AppRepo:  &stubApplicationRepository{},
		Augment:  nil,
		Defaults: defaults,
		Tailor:   nil,
	}

	req := callToolRequest("get_score", map[string]any{
		"text":    "Software Engineer role at Acme Corp",
		"channel": "COLD",
	})

	result := mcpserver.HandleGetScore(context.Background(), &req, &deps)

	text := extractText(t, result)
	var payload model.PipelineResult
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Status != "error" {
		t.Errorf("expected status error when JD is empty, got %q", payload.Status)
	}
	if !strings.Contains(payload.Error, "could not extract a job description") {
		t.Errorf("expected actionable error message, got: %s", payload.Error)
	}
}
