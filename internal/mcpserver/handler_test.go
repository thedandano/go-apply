package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/onboarding"
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

type stubResumeRepo struct{}

var _ port.ResumeRepository = (*stubResumeRepo)(nil)

func (s *stubResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return []model.ResumeFile{{Label: "main", Path: "/fake/main.txt"}}, nil
}

func (s *stubResumeRepo) LoadSections(_ string) (model.SectionMap, error) {
	return model.SectionMap{}, model.ErrSectionsMissing
}

func (s *stubResumeRepo) SaveSections(_ string, _ model.SectionMap) error { return nil } //nolint:gocritic // hugeParam: interface constraint

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

func TestHandleUpdateConfig_ValidKey_ReturnsSuccess(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	req := callToolRequest("update_config", map[string]any{
		"key":   "log_level",
		"value": "debug",
	})
	cfg := &config.Config{}
	result := mcpserver.HandleUpdateConfig(context.Background(), &req, cfg)
	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if resp["error"] != "" {
		t.Errorf("unexpected error: %s", resp["error"])
	}
}

// ── handleGetConfigWith tests ─────────────────────────────────────────────────

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

	// Create accomplishments.json at dataDir root with non-empty onboard_text.
	accJSON := `{"schema_version":"1","onboard_text":"accomplished things","created_stories":[]}`
	accomplishmentsPath := filepath.Join(tmpDir, "accomplishments.json")
	if err := os.WriteFile(accomplishmentsPath, []byte(accJSON), 0o644); err != nil {
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
		t.Error("profile.has_accomplishments = false, want true when accomplishments.json has onboard_text or stories")
	}
}

func TestHandleGetConfigWithProfile_OnboardedFalse_WhenNoResumes(t *testing.T) {
	cfg := &config.Config{}

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

// TestBuildProfileStatus_HasAccomplishments_ParseError verifies that malformed accomplishments.json
// causes has_accomplishments to be false and a warning to be logged.
func TestBuildProfileStatus_HasAccomplishments_ParseError(t *testing.T) {
	dir := t.TempDir()

	// Write malformed JSON to accomplishments.json.
	if err := os.WriteFile(filepath.Join(dir, "accomplishments.json"), []byte("{bad json"), 0o600); err != nil {
		t.Fatalf("write accomplishments.json: %v", err)
	}

	// Capture slog output at Warn level.
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	result := mcpserver.HandleGetConfigWithProfileAndFiles(&config.Config{}, dir)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	profile, _ := resp["profile"].(map[string]interface{})
	if profile == nil {
		t.Fatal("profile missing from response")
	}

	hasAcc, _ := profile["has_accomplishments"].(bool)
	if hasAcc {
		t.Error("has_accomplishments = true; want false when accomplishments.json is malformed")
	}

	if !strings.Contains(buf.String(), "WARN") {
		t.Errorf("expected warning log for parse error, got: %s", buf.String())
	}
}

func TestHandleGetConfigWithProfile_PreservesConfigFields(t *testing.T) {
	cfg := &config.Config{}

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

	// orchestrator.* keys were renamed to llm.* — verify the old names are absent.
	for _, key := range []string{"orchestrator.base_url", "orchestrator.model", "orchestrator.api_key"} {
		if _, found := response[key]; found {
			t.Errorf("get_config must not expose %q (renamed to llm.*)", key)
		}
	}

	// log_level should be present (it is in MCPKeys).
	if _, ok := response["log_level"]; !ok {
		t.Error("response missing 'log_level' key")
	}
}

// TestBuildProfileStatus_Stale_WhenSourceNewerThanProfile verifies stale=true and stale_files
// are reported in the profile object when an accomplishments file is newer than the compiled profile.
func TestBuildProfileStatus_Stale_WhenSourceNewerThanProfile(t *testing.T) {
	dir := t.TempDir()

	// Write profile-compiled.json with a past CompiledAt.
	past := time.Now().Add(-10 * time.Minute)
	prof := model.CompiledProfile{SchemaVersion: "1", CompiledAt: past}
	data, _ := json.Marshal(prof)
	if err := os.WriteFile(filepath.Join(dir, "profile-compiled.json"), data, 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	// Write accomplishments.json with an mtime AFTER CompiledAt.
	accPath := filepath.Join(dir, "accomplishments.json")
	if err := os.WriteFile(accPath, []byte(`{"schema_version":"1","onboard_text":"story","created_stories":[]}`), 0o600); err != nil {
		t.Fatalf("write accomplishments: %v", err)
	}
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(accPath, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	result := mcpserver.HandleGetConfigWithProfileAndFiles(&config.Config{}, dir)
	text := extractText(t, result)
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	profile, _ := resp["profile"].(map[string]interface{})
	if profile == nil {
		t.Fatal("profile missing from response")
	}
	stale, _ := profile["stale"].(bool)
	if !stale {
		t.Error("stale=false; want true when source file is newer than profile")
	}
	staleFiles, _ := profile["stale_files"].([]interface{})
	if len(staleFiles) == 0 {
		t.Error("stale_files empty; want at least one entry")
	}
}

// TestBuildProfileStatus_Stale_WhenProfileAbsent verifies stale is true when
// no compiled profile exists — NeedsCompilation returns (true,nil,nil) for absent profile.
func TestBuildProfileStatus_Stale_WhenProfileAbsent(t *testing.T) {
	dir := t.TempDir()

	result := mcpserver.HandleGetConfigWithProfileAndFiles(&config.Config{}, dir)
	text := extractText(t, result)
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	profile, _ := resp["profile"].(map[string]interface{})
	if profile == nil {
		t.Fatal("profile missing from response")
	}
	stale, _ := profile["stale"].(bool)
	if !stale {
		t.Error("stale=false; want true when no compiled profile exists")
	}
	staleFiles, _ := profile["stale_files"].([]interface{})
	if staleFiles == nil {
		t.Error("stale_files missing or null; want empty array when profile absent")
	}
}

// TestHandleOnboardUser_NeedsCompile verifies that onboard_user always returns needs_compile: true.
func TestHandleOnboardUser_NeedsCompile(t *testing.T) {
	svc := &stubOnboarder{
		result: model.OnboardResult{Stored: []string{"resume:backend"}},
	}
	req := callToolRequest("onboard_user", map[string]any{
		"resume_content": "resume text",
		"resume_label":   "backend",
	})
	result := mcpserver.HandleOnboardUser(context.Background(), &req, svc)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v — raw: %s", err, text)
	}
	if resp["needs_compile"] != true {
		t.Errorf("needs_compile = %v; want true", resp["needs_compile"])
	}
}

// TestHandleCreateStory_NeedsCompile verifies that create_story returns needs_compile: true
// after successfully saving the story.
func TestHandleCreateStory_NeedsCompile(t *testing.T) {
	creator := &stubCreator{
		out: model.StoryOutput{StoryID: "1"},
	}
	args := map[string]any{
		"skill":      "Go",
		"story_type": "technical",
		"job_title":  "SWE",
		"situation":  "legacy system slowing releases",
		"behavior":   "rewrote the pipeline in Go",
		"impact":     "20% faster deployments",
	}
	result := mcpserver.HandleCreateStoryWith(context.Background(), args, creator)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v — raw: %s", err, text)
	}
	if resp["needs_compile"] != true {
		t.Errorf("needs_compile = %v; want true", resp["needs_compile"])
	}
	if resp["story_id"] != "1" {
		t.Errorf("story_id = %v; want \"1\"", resp["story_id"])
	}
}

// TestOnboardThenHasAccomplishments verifies the end-to-end contract:
// HandleOnboardUserWith writes accomplishments.json, and HandleGetConfigWithProfileAndFiles
// subsequently reports has_accomplishments=true.
func TestOnboardThenHasAccomplishments(t *testing.T) {
	dir := t.TempDir()
	svc := onboarding.New(dir, slog.Default())
	req := callToolRequest("onboard_user", map[string]any{
		"resume_content":  "resume text",
		"resume_label":    "backend",
		"accomplishments": "Led migration to microservices",
	})
	onboardResult := mcpserver.HandleOnboardUserWith(context.Background(), &req, svc, dir)
	onboardText := extractText(t, onboardResult)
	var onboardResp map[string]interface{}
	if err := json.Unmarshal([]byte(onboardText), &onboardResp); err != nil {
		t.Fatalf("onboard response parse: %v — raw: %s", err, onboardText)
	}
	if _, hasErr := onboardResp["error"]; hasErr {
		t.Fatalf("onboard_user returned error: %s", onboardText)
	}

	result := mcpserver.HandleGetConfigWithProfileAndFiles(&config.Config{}, dir)
	text := extractText(t, result)
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("get_config response parse: %v — raw: %s", err, text)
	}
	profile, ok := resp["profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("profile is not a map: %T", resp["profile"])
	}
	if profile["has_accomplishments"] != true {
		t.Errorf("has_accomplishments = %v; want true after onboard_user with accomplishments text", profile["has_accomplishments"])
	}
}

// TestCreateStoryThenCompileProfile verifies the end-to-end contract: the story_id returned
// by create_story, when passed as Source to compile_profile, populates Story.SourceFile
// in the compiled profile saved to disk.
func TestCreateStoryThenCompileProfile(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Step 1: create_story returns story_id "0" (via stub).
	creator := &stubCreator{out: model.StoryOutput{StoryID: "0"}}
	storyResult := mcpserver.HandleCreateStoryWith(ctx, baseStoryArgs(), creator)
	storyText := extractText(t, storyResult)
	var storyResp map[string]interface{}
	if err := json.Unmarshal([]byte(storyText), &storyResp); err != nil {
		t.Fatalf("story response parse: %v — raw: %s", err, storyText)
	}
	storyID, _ := storyResp["story_id"].(string)
	if storyID == "" {
		t.Fatalf("story_id empty in create_story response: %s", storyText)
	}

	// Step 2: pass story_id as Source to compile_profile.
	compileResult := mcpserver.HandleCompileProfileWith(ctx, dir, model.AssembleInput{
		Skills: []string{"Go"},
		Stories: []model.AssembleStory{
			{Accomplishment: "rewrote the handler", Tags: []string{"Go"}, Source: storyID},
		},
	})
	compileText := extractText(t, compileResult)
	var compileResp map[string]interface{}
	if err := json.Unmarshal([]byte(compileText), &compileResp); err != nil {
		t.Fatalf("compile response parse: %v — raw: %s", err, compileText)
	}
	if _, hasErr := compileResp["error"]; hasErr {
		t.Fatalf("compile_profile returned error: %s", compileText)
	}

	// Step 3: load the saved profile and assert SourceFile == storyID.
	raw, err := os.ReadFile(filepath.Join(dir, "profile-compiled.json"))
	if err != nil {
		t.Fatalf("read profile-compiled.json: %v", err)
	}
	var profile model.CompiledProfile
	if err := json.Unmarshal(raw, &profile); err != nil {
		t.Fatalf("parse profile-compiled.json: %v", err)
	}
	if len(profile.Stories) != 1 {
		t.Fatalf("stories count = %d; want 1", len(profile.Stories))
	}
	if profile.Stories[0].SourceFile != storyID {
		t.Errorf("Story.SourceFile = %q; want %q (story_id should flow through to compiled profile)", profile.Stories[0].SourceFile, storyID)
	}
}
