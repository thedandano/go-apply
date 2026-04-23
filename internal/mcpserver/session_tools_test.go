package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/tailorllm"
)

// stubApplyConfigForSession returns an ApplyConfig with all stubs and no Presenter set.
// The handlers set the presenter internally.
func stubApplyConfigForSession() pipeline.ApplyConfig {
	return pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       &stubLLMClient{},
		Scorer:    &stubScorer{},
		CLGen:     nil,
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubApplicationRepository{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}
}

// ── HandleLoadJD tests ────────────────────────────────────────────────────────

func TestHandleLoadJD_BothArgs_ReturnsError(t *testing.T) {
	req := callToolRequest("load_jd", map[string]any{
		"jd_url":      "https://example.com/job",
		"jd_raw_text": "raw text",
	})
	result := mcpserver.HandleLoadJD(context.Background(), &req)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
}

func TestHandleLoadJD_NoArgs_ReturnsError(t *testing.T) {
	req := callToolRequest("load_jd", map[string]any{})
	result := mcpserver.HandleLoadJD(context.Background(), &req)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
}

func TestHandleLoadJDWithConfig_TextInput_ReturnsSession(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Senior Go engineer wanted. Kubernetes required.",
	})
	result := mcpserver.HandleLoadJDWithConfig(context.Background(), &req, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	if env["session_id"] == "" {
		t.Error("expected non-empty session_id")
	}
	if env["next_action"] != "extract_keywords" {
		t.Errorf("next_action = %v, want extract_keywords", env["next_action"])
	}
	data, _ := env["data"].(map[string]any)
	if data == nil || data["jd_text"] == "" {
		t.Error("expected data.jd_text in response")
	}
}

// ── HandleSubmitKeywords tests ────────────────────────────────────────────────

func TestHandleSubmitKeywordsWithConfig_MissingSession_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_keywords", map[string]any{
		"session_id": "nonexistent-id",
		"jd_json":    `{"title":"SWE","required":["go"],"preferred":["docker"]}`,
	})
	result := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "session_not_found" {
		t.Errorf("expected code session_not_found, got %v", errObj)
	}
}

func TestHandleSubmitKeywordsWithConfig_InvalidJD_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()

	// Create a session first.
	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Go engineer role",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd response not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	// Submit malformed JSON.
	req := callToolRequest("submit_keywords", map[string]any{
		"session_id": sessionID,
		"jd_json":    "not-valid-json",
	})
	result := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
}

func TestHandleSubmitKeywordsWithConfig_HappyPath_ReturnsScores(t *testing.T) {
	cfg := stubApplyConfigForSession()

	// Load JD.
	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Senior Go engineer. Required: go, kubernetes.",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd response not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	// Submit keywords.
	jdJSON := `{"title":"Go Engineer","company":"Acme","required":["go","kubernetes"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{
		"session_id": sessionID,
		"jd_json":    jdJSON,
	})
	kwResult := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{YearsOfExperience: 5})
	kwText := extractText(t, kwResult)

	var kwEnv map[string]any
	if err := json.Unmarshal([]byte(kwText), &kwEnv); err != nil {
		t.Fatalf("submit_keywords response not JSON: %v — raw: %s", err, kwText)
	}
	if kwEnv["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", kwEnv["status"], kwText)
	}
	if kwEnv["session_id"] != sessionID {
		t.Errorf("session_id = %v, want %v", kwEnv["session_id"], sessionID)
	}
	data, _ := kwEnv["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in submit_keywords response")
	}
	if _, ok := data["scores"]; !ok {
		t.Error("expected scores in data")
	}
	if _, ok := data["best_resume"]; !ok {
		t.Error("expected best_resume in data")
	}
	nextAction, _ := kwEnv["next_action"].(string)
	validActions := map[string]bool{"cover_letter": true, "tailor_t1": true, "advise_skip": true}
	if !validActions[nextAction] {
		t.Errorf("next_action = %q, want one of: cover_letter, tailor_t1, advise_skip", nextAction)
	}

	// Verify extracted keywords are echoed back in the response.
	ekRaw, ok := data["extracted_keywords"].(map[string]any)
	if !ok || ekRaw == nil {
		t.Fatal("expected extracted_keywords in data")
	}
	if ekRaw["title"] != "Go Engineer" {
		t.Errorf("extracted_keywords.title = %v, want Go Engineer", ekRaw["title"])
	}
	required, _ := ekRaw["required"].([]any)
	if len(required) == 0 {
		t.Error("expected at least one item in extracted_keywords.required")
	}
}

// ── HandleFinalize tests ──────────────────────────────────────────────────────

func TestHandleFinalizeWithConfig_MissingSession_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("finalize", map[string]any{
		"session_id": "nonexistent",
	})
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &req, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
}

func TestHandleFinalizeWithConfig_WrongState_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()

	// Load JD but don't submit keywords — session is in stateLoaded.
	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Go engineer role",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd response not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	req := callToolRequest("finalize", map[string]any{"session_id": sessionID})
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &req, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
	if !strings.Contains(text, "invalid_state") {
		t.Errorf("expected invalid_state code, got: %s", text)
	}
}

func TestHandleFinalizeWithConfig_HappyPath(t *testing.T) {
	cfg := stubApplyConfigForSession()

	// Full flow: load_jd → submit_keywords → finalize.
	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Senior Go engineer. Required: go.",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	kwReq := callToolRequest("submit_keywords", map[string]any{
		"session_id": sessionID,
		"jd_json":    `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`,
	})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	finReq := callToolRequest("finalize", map[string]any{
		"session_id":   sessionID,
		"cover_letter": "Dear Hiring Manager...",
	})
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &finReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("finalize response not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in finalize response")
	}
	if data["cover_letter"] != "Dear Hiring Manager..." {
		t.Errorf("cover_letter = %v, want 'Dear Hiring Manager...'", data["cover_letter"])
	}
}

func TestHandleFinalizeWithConfig_SummaryIncluded(t *testing.T) {
	cfg := stubApplyConfigForSession()

	// Full flow: load_jd → submit_keywords (1 required, 0 preferred) → finalize.
	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Senior Go engineer. Required: go.",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	kwReq := callToolRequest("submit_keywords", map[string]any{
		"session_id": sessionID,
		"jd_json":    `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`,
	})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	const coverLetter = "Dear Hiring Manager, I am applying..."
	finReq := callToolRequest("finalize", map[string]any{
		"session_id":   sessionID,
		"cover_letter": coverLetter,
	})
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &finReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("finalize response not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in finalize response")
	}
	summary, _ := data["summary"].(map[string]any)
	if summary == nil {
		t.Fatalf("expected summary in data, got: %s", text)
	}
	if summary["keywords_required"] != float64(1) {
		t.Errorf("keywords_required = %v, want 1", summary["keywords_required"])
	}
	if summary["keywords_preferred"] != float64(0) {
		t.Errorf("keywords_preferred = %v, want 0", summary["keywords_preferred"])
	}
	if summary["cover_letter_chars"] != float64(len(coverLetter)) {
		t.Errorf("cover_letter_chars = %v, want %d", summary["cover_letter_chars"], len(coverLetter))
	}
	if _, ok := summary["resumes_scored"]; !ok {
		t.Error("expected resumes_scored in summary")
	}
	if _, ok := summary["best_resume"]; !ok {
		t.Error("expected best_resume in summary")
	}
	if _, ok := summary["best_score"]; !ok {
		t.Error("expected best_score in summary")
	}
}

// ── nextActionAfterT1 tests ───────────────────────────────────────────────────

func TestNextActionAfterT1(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0.0, "tailor_t2"},
		{69.9, "tailor_t2"},
		{70.0, "cover_letter"},
		{100.0, "cover_letter"},
	}
	for _, c := range cases {
		got := mcpserver.NextActionAfterT1(c.score)
		if got != c.want {
			t.Errorf("NextActionAfterT1(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}

// ── HandleSubmitTailorT1 tests ────────────────────────────────────────────────

func TestHandleSubmitTailorT1_SessionNotFound_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": "no-such-session",
		"skill_adds": `["Go","Kubernetes"]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
}

func TestHandleSubmitTailorT1_WrongState_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	// load_jd only — state stays stateLoaded
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Go engineer role"})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	_ = json.Unmarshal([]byte(loadText), &loadEnv)
	sessionID, _ := loadEnv["session_id"].(string)

	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"skill_adds": `["Go"]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_state") {
		t.Errorf("expected invalid_state error, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_HappyPath_ReturnsNewScore(t *testing.T) {
	cfg := stubApplyConfigForSession()

	// load_jd
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer. Skills: go, kubernetes."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	_ = json.Unmarshal([]byte(loadText), &loadEnv)
	sessionID, _ := loadEnv["session_id"].(string)

	// submit_keywords
	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go","kubernetes"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	// submit_tailor_t1
	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"skill_adds": `["Terraform","gRPC"]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{})
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil || data["new_score"] == nil {
		t.Errorf("expected new_score in data, got: %s", text)
	}
	// added_keywords may be null when the stub resume has no Skills section header
	if _, ok := data["added_keywords"]; !ok {
		t.Errorf("expected added_keywords key in data, got: %s", text)
	}
	// skills_section_found must always be present so the orchestrator can distinguish
	// "T1 was a no-op because no skills header exists" from "T1 ran successfully".
	found, ok := data["skills_section_found"].(bool)
	if !ok {
		t.Errorf("expected skills_section_found bool in data, got: %s", text)
	}
	// The stub resume has no skills header, so this must be false — confirms the
	// orchestrator-visible signal matches the internal state.
	if found {
		t.Errorf("expected skills_section_found=false for stub resume without skills header, got true — full: %s", text)
	}
}

func TestHandleSubmitTailorT1_MissingSkillAdds_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": "any-id",
		// skill_adds missing
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "missing_skill_adds") {
		t.Errorf("expected missing_skill_adds error, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_InvalidSkillAddsJSON_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": "any-id",
		"skill_adds": `not-valid-json`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_skill_adds") {
		t.Errorf("expected invalid_skill_adds error, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_EmptySkillAdds_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": "any-id",
		"skill_adds": `[]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "empty_skill_adds") {
		t.Errorf("expected empty_skill_adds error, got: %s", text)
	}
}

// stubScorerFailAfter succeeds on the first N calls then returns an error.
type stubScorerFailAfter struct {
	callsLeft atomic.Int32
}

var _ port.Scorer = (*stubScorerFailAfter)(nil)

func newScorerFailAfter(n int) *stubScorerFailAfter {
	s := &stubScorerFailAfter{}
	s.callsLeft.Store(int32(n)) //nolint:gosec // n is small test constant
	return s
}

func (s *stubScorerFailAfter) Score(_ *model.ScorerInput) (model.ScoreResult, error) {
	if s.callsLeft.Add(-1) >= 0 {
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
	return model.ScoreResult{}, errors.New("rescore: injected failure")
}

// stubDocumentLoaderLong returns a resume text that is longer than 2048 bytes
// so that PayloadAttr truncation is observable when verbose=false.
type stubDocumentLoaderLong struct{}

var _ port.DocumentLoader = (*stubDocumentLoaderLong)(nil)

// longResumePrefix is a substring unique to the long stub resume.
// It must not contain a newline so it matches inside the JSON-encoded result payload.
const longResumePrefix = "golang experience senior engineer 5 years"

func longResumeText() string {
	// Build a string well over 2048 bytes so truncation triggers at verbose=false.
	var b strings.Builder
	b.WriteString(longResumePrefix)
	line := "Go developer with extensive backend systems experience and cloud infrastructure expertise.\n"
	for b.Len() < 3000 {
		b.WriteString(line)
	}
	return b.String()
}

func (s *stubDocumentLoaderLong) Load(_ string) (string, error) { return longResumeText(), nil }
func (s *stubDocumentLoaderLong) Supports(_ string) bool        { return true }

// stubApplyConfigForSessionLong returns a config that uses the long-text loader.
func stubApplyConfigForSessionLong() pipeline.ApplyConfig {
	return pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       &stubLLMClient{},
		Scorer:    &stubScorer{},
		CLGen:     nil,
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoaderLong{},
		AppRepo:   &stubApplicationRepository{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}
}

// captureDefaultLogger installs a JSON slog handler writing to buf as the default
// logger and returns a cleanup function that restores the previous default.
func captureDefaultLogger(buf *bytes.Buffer) func() {
	prev := slog.Default()
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))
	return func() { slog.SetDefault(prev) }
}

// fullT1Flow runs load_jd → submit_keywords → submit_tailor_t1 and returns the T1 response text.
func fullT1Flow(t *testing.T, cfg *pipeline.ApplyConfig, skillAdds string) string {
	t.Helper()

	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer. Skills: go."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, cfg, &config.Config{})

	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"skill_adds": skillAdds,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, cfg, &config.Config{})
	return extractText(t, result)
}

// ── Task 2.4: tailored_text in response data ──────────────────────────────────

func TestHandleSubmitTailorT1_HappyPath_ResponseContainsTailoredText(t *testing.T) {
	cfg := stubApplyConfigForSession()
	text := fullT1Flow(t, &cfg, `["Terraform","gRPC"]`)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}

	// The stub resume has no skills header, so T1 is a no-op and tailored_text ==
	// the original resume text returned by stubDocumentLoader.
	tailoredText, ok := data["tailored_text"].(string)
	if !ok {
		t.Fatalf("expected tailored_text string in data, got: %s", text)
	}
	const wantText = "golang experience senior engineer 5 years"
	if tailoredText != wantText {
		t.Errorf("tailored_text = %q, want %q", tailoredText, wantText)
	}
}

// ── Task 2.3: verbose-gated debug log ─────────────────────────────────────────

func TestHandleSubmitTailorT1_VerboseLog_ResultPayloadGated(t *testing.T) {
	// Use a long resume so truncation is observable at verbose=false.
	longText := longResumeText()
	// Sanity: long resume must exceed the 2048-byte payload limit.
	if len(longText) <= 2048 {
		t.Fatalf("longResumeText() is only %d bytes — must exceed 2048 for truncation test", len(longText))
	}

	tests := []struct {
		name          string
		verbose       bool
		wantTruncated bool
	}{
		{"verbose=true keeps full payload", true, false},
		{"verbose=false truncates payload", false, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := stubApplyConfigForSessionLong()

			var buf bytes.Buffer
			restore := captureDefaultLogger(&buf)
			t.Cleanup(restore)

			logger.SetVerbose(tc.verbose)
			t.Cleanup(func() { logger.SetVerbose(false) })

			text := fullT1Flow(t, &cfg, `["Terraform"]`)

			var env map[string]any
			if err := json.Unmarshal([]byte(text), &env); err != nil {
				t.Fatalf("not JSON: %v", err)
			}
			if env["status"] != "ok" {
				t.Fatalf("status = %v, want ok — full: %s", env["status"], text)
			}

			// Parse captured slog output: find the "mcp tool result" line for submit_tailor_t1.
			logOutput := buf.String()
			foundResultLine := false
			for _, line := range strings.Split(logOutput, "\n") {
				if line == "" {
					continue
				}
				var entry map[string]any
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					continue
				}
				if entry["msg"] != "mcp tool result" || entry["tool"] != "submit_tailor_t1" {
					continue
				}
				foundResultLine = true
				resultPayload, _ := entry["result"].(string)
				// Truncate produces " … [N bytes omitted] … " — search for the
				// number-agnostic substring so we don't hard-code the byte count.
				truncationMarker := "bytes omitted"
				if tc.wantTruncated {
					if !strings.Contains(resultPayload, truncationMarker) {
						t.Errorf("verbose=false: expected truncation marker in result payload (len=%d), prefix: %q",
							len(resultPayload), resultPayload[:min(len(resultPayload), 200)])
					}
				} else {
					if strings.Contains(resultPayload, truncationMarker) {
						t.Errorf("verbose=true: unexpected truncation marker in result payload")
					}
					// Verify the tailored text is transitively visible in the payload.
					if !strings.Contains(resultPayload, longResumePrefix) {
						t.Errorf("verbose=true: expected resume prefix %q in result payload (len=%d)",
							longResumePrefix, len(resultPayload))
					}
				}
				break
			}
			if !foundResultLine {
				t.Error("'mcp tool result' debug log line not found in captured output")
			}
		})
	}
}

// ── Task 2.4: info log carries tailored_text_bytes / tailored_text_lines ──────

func TestHandleSubmitTailorT1_InfoLog_ContainsTextBytesAndLines(t *testing.T) {
	cfg := stubApplyConfigForSession()

	var buf bytes.Buffer
	restore := captureDefaultLogger(&buf)
	t.Cleanup(restore)

	text := fullT1Flow(t, &cfg, `["Terraform"]`)
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok", env["status"])
	}

	logOutput := buf.String()
	foundInfoLine := false
	for _, line := range strings.Split(logOutput, "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["msg"] != "tailor T1 complete" {
			continue
		}
		foundInfoLine = true
		if _, ok := entry["tailored_text_bytes"]; !ok {
			t.Error("info log missing tailored_text_bytes")
		}
		if _, ok := entry["tailored_text_lines"]; !ok {
			t.Error("info log missing tailored_text_lines")
		}
		break
	}
	if !foundInfoLine {
		t.Error("'tailor T1 complete' info log line not found in captured output")
	}
}

// ── Task 2.5: error paths do not contain tailored_text ────────────────────────

func TestHandleSubmitTailorT1_BadInput_ErrorEnvelopeHasNoTailoredText(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": "any-id",
		"skill_adds": `not-valid-json`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Fatalf("status = %v, want error", env["status"])
	}
	if strings.Contains(text, "tailored_text") {
		t.Errorf("error envelope must not contain tailored_text, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_RescoreFailure_ErrorEnvelopeHasNoTailoredText(t *testing.T) {
	// Use a scorer that succeeds on the first N calls (one per resume in stubResumeRepo)
	// during submit_keywords, then fails on the rescore call in submit_tailor_t1.
	failingScorer := newScorerFailAfter(1)
	cfg := pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       &stubLLMClient{},
		Scorer:    failingScorer,
		CLGen:     nil,
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubApplicationRepository{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}

	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"skill_adds": `["Terraform"]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{})
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Fatalf("status = %v, want error (rescore should fail) — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "rescore_failed" {
		t.Errorf("expected code rescore_failed, got: %v", errObj)
	}
	if strings.Contains(text, "tailored_text") {
		t.Errorf("error envelope must not contain tailored_text, got: %s", text)
	}
}

// ── HandleSubmitTailorT2 tests ────────────────────────────────────────────────

func TestHandleSubmitTailorT2_SessionNotFound_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id":      "no-such-session",
		"bullet_rewrites": `[{"original":"old","replacement":"new"}]`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj["code"] != "session_not_found" {
		t.Errorf("code = %v, want session_not_found", errObj["code"])
	}
}

func TestHandleSubmitTailorT2_WrongState_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	// load_jd only — state stays stateLoaded
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Go engineer role"})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	_ = json.Unmarshal([]byte(loadText), &loadEnv)
	sessionID, _ := loadEnv["session_id"].(string)

	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id":      sessionID,
		"bullet_rewrites": `[{"original":"old","replacement":"new"}]`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_state") {
		t.Errorf("expected invalid_state error, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_MissingBulletRewrites_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": "any-id",
		// bullet_rewrites missing
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "missing_bullet_rewrites") {
		t.Errorf("expected missing_bullet_rewrites error, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_InvalidBulletRewritesJSON_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id":      "any-id",
		"bullet_rewrites": `not-valid-json`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_bullet_rewrites") {
		t.Errorf("expected invalid_bullet_rewrites error, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_EmptyBulletRewrites_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id":      "any-id",
		"bullet_rewrites": `[]`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "empty_bullet_rewrites") {
		t.Errorf("expected empty_bullet_rewrites error, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_HappyPath_ReturnsNewScore(t *testing.T) {
	cfg := stubApplyConfigForSession()

	// load_jd
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer. Skills: go."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	_ = json.Unmarshal([]byte(loadText), &loadEnv)
	sessionID, _ := loadEnv["session_id"].(string)

	// submit_keywords
	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	// submit_tailor_t2 (directly after scored, skipping T1)
	rewrites := `[{"original":"golang experience senior engineer 5 years","replacement":"golang experience senior engineer 5 years, Kubernetes"}]`
	t2Req := callToolRequest("submit_tailor_t2", map[string]any{"session_id": sessionID, "bullet_rewrites": rewrites})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &t2Req, &cfg, &config.Config{})
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok", env["status"])
	}
	if env["next_action"] != "cover_letter" {
		t.Errorf("next_action = %v, want cover_letter", env["next_action"])
	}
	data, _ := env["data"].(map[string]any)
	if data == nil || data["new_score"] == nil || data["substitutions_made"] == nil {
		t.Errorf("expected new_score and substitutions_made in data, got: %s", text)
	}
}

// fullT2Flow runs load_jd → submit_keywords → submit_tailor_t2 and returns the T2 response text.
func fullT2Flow(t *testing.T, cfg *pipeline.ApplyConfig, rewrites string) string {
	t.Helper()

	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer. Skills: go."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, cfg, &config.Config{})

	t2Req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id":      sessionID,
		"bullet_rewrites": rewrites,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &t2Req, cfg, &config.Config{})
	return extractText(t, result)
}

// ── Task 3.3: tailored_text in T2 response data ───────────────────────────────

func TestHandleSubmitTailorT2_HappyPath_ResponseContainsTailoredText(t *testing.T) {
	cfg := stubApplyConfigForSession()
	// The stub loader returns "golang experience senior engineer 5 years".
	// The rewrite replaces it verbatim, so tailored_text should equal the replacement.
	const rewrites = `[{"original":"golang experience senior engineer 5 years","replacement":"golang experience senior engineer 5 years, Kubernetes"}]`
	text := fullT2Flow(t, &cfg, rewrites)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}

	tailoredText, ok := data["tailored_text"].(string)
	if !ok {
		t.Fatalf("expected tailored_text string in data, got: %s", text)
	}
	const wantText = "golang experience senior engineer 5 years, Kubernetes"
	if tailoredText != wantText {
		t.Errorf("tailored_text = %q, want %q", tailoredText, wantText)
	}
}

// ── Task 3.2: info log carries tailored_text_bytes / tailored_text_lines ──────

func TestHandleSubmitTailorT2_InfoLog_ContainsTextBytesAndLines(t *testing.T) {
	cfg := stubApplyConfigForSession()

	var buf bytes.Buffer
	restore := captureDefaultLogger(&buf)
	t.Cleanup(restore)

	const rewrites = `[{"original":"golang experience senior engineer 5 years","replacement":"golang experience senior engineer 5 years, Kubernetes"}]`
	text := fullT2Flow(t, &cfg, rewrites)
	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok", env["status"])
	}

	logOutput := buf.String()
	foundInfoLine := false
	for _, line := range strings.Split(logOutput, "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["msg"] != "tailor T2 complete" {
			continue
		}
		foundInfoLine = true
		if _, ok := entry["tailored_text_bytes"]; !ok {
			t.Error("info log missing tailored_text_bytes")
		}
		if _, ok := entry["tailored_text_lines"]; !ok {
			t.Error("info log missing tailored_text_lines")
		}
		break
	}
	if !foundInfoLine {
		t.Error("'tailor T2 complete' info log line not found in captured output")
	}
}

// ── Task 3.4: error paths do not contain tailored_text ────────────────────────

func TestHandleSubmitTailorT2_BadInput_ErrorEnvelopeHasNoTailoredText(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id":      "any-id",
		"bullet_rewrites": `not-valid-json`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Fatalf("status = %v, want error", env["status"])
	}
	if strings.Contains(text, "tailored_text") {
		t.Errorf("error envelope must not contain tailored_text, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_RescoreFailure_ErrorEnvelopeHasNoTailoredText(t *testing.T) {
	// Use a scorer that succeeds on the first N calls (one per resume in stubResumeRepo)
	// during submit_keywords, then fails on the rescore call in submit_tailor_t2.
	failingScorer := newScorerFailAfter(1)
	cfg := pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       &stubLLMClient{},
		Scorer:    failingScorer,
		CLGen:     nil,
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubApplicationRepository{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}

	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	t2Req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id":      sessionID,
		"bullet_rewrites": `[{"original":"golang experience senior engineer 5 years","replacement":"golang experience senior engineer 5 years, Kubernetes"}]`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &t2Req, &cfg, &config.Config{})
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Fatalf("status = %v, want error (rescore should fail) — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "rescore_failed" {
		t.Errorf("expected code rescore_failed, got: %v", errObj)
	}
	if strings.Contains(text, "tailored_text") {
		t.Errorf("error envelope must not contain tailored_text, got: %s", text)
	}
}

// ── nextActionFromScore tests ─────────────────────────────────────────────────

func TestNextActionFromScore(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0.0, "advise_skip"},
		{30.0, "advise_skip"},
		{39.9, "advise_skip"},
		{40.0, "tailor_t1"},
		{49.8, "tailor_t1"}, // the reported misfire: 49.8/100 must be tailor_t1
		{55.0, "tailor_t1"},
		{69.9, "tailor_t1"},
		{70.0, "cover_letter"},
		{90.0, "cover_letter"},
		{100.0, "cover_letter"},
	}
	for _, c := range cases {
		got := mcpserver.NextActionFromScore(c.score)
		if got != c.want {
			t.Errorf("NextActionFromScore(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}

// ── HandleTailorBegin / HandleTailorSubmit tests (T014–T020) ─────────────────

func TestHandleTailorBegin_HappyPath(t *testing.T) {
	store := mcpserver.NewTailorSessionStore()
	req := callToolRequest("tailor_begin", map[string]any{
		"resume_text":          "resume content",
		"accomplishments_text": "my accomplishments",
		"jd":                   "{}",
		"score_before":         "{}",
		"options":              "{}",
	})
	result := mcpserver.HandleTailorBeginWithStore(context.Background(), &req, store, "mock-prompt-body")
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}
	sessionID, _ := data["session_id"].(string)
	if sessionID == "" {
		t.Error("expected non-empty session_id in data")
	}
	promptBundle, _ := data["prompt_bundle"].(string)
	if promptBundle == "" {
		t.Error("expected non-empty prompt_bundle in data")
	}
}

func TestHandleTailorBegin_MissingResumeText(t *testing.T) {
	store := mcpserver.NewTailorSessionStore()
	req := callToolRequest("tailor_begin", map[string]any{
		"resume_text":          "",
		"accomplishments_text": "my accomplishments",
		"jd":                   "{}",
		"score_before":         "{}",
		"options":              "{}",
	})
	result := mcpserver.HandleTailorBeginWithStore(context.Background(), &req, store, "mock-prompt-body")
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "invalid_input" {
		t.Errorf("expected code invalid_input, got: %v", errObj)
	}
}

func TestHandleTailorSubmit_HappyPath(t *testing.T) {
	store := mcpserver.NewTailorSessionStore()
	input := &model.TailorInput{ResumeText: "original resume"}
	id, err := store.Open("bundle", input, 5*time.Second)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}

	changelog := []map[string]any{
		{"kind": "skill_add", "tier": "tier_1", "keyword": "Go"},
		{"kind": "skip", "tier": "tier_1", "keyword": "Rust", "reason": "no_basis_found"},
	}
	changelogJSON, err := json.Marshal(changelog)
	if err != nil {
		t.Fatalf("marshal changelog: %v", err)
	}

	req := callToolRequest("tailor_submit", map[string]any{
		"session_id":    id,
		"tailored_text": "tailored content",
		"changelog":     string(changelogJSON),
	})
	result := mcpserver.HandleTailorSubmitWithStore(context.Background(), &req, store)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}
	if data["acknowledged"] != true {
		t.Errorf("acknowledged = %v, want true", data["acknowledged"])
	}

	waitResult, waitErr := store.Wait(context.Background(), id)
	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if waitResult.TailoredText != "tailored content" {
		t.Errorf("TailoredText = %q, want %q", waitResult.TailoredText, "tailored content")
	}
	if len(waitResult.Changelog) != 2 {
		t.Errorf("len(Changelog) = %d, want 2", len(waitResult.Changelog))
	}
}

func TestHandleTailorSubmit_UnknownSession(t *testing.T) {
	store := mcpserver.NewTailorSessionStore()
	req := callToolRequest("tailor_submit", map[string]any{
		"session_id":    "nonexistent",
		"tailored_text": "x",
		"changelog":     "[]",
	})
	result := mcpserver.HandleTailorSubmitWithStore(context.Background(), &req, store)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "unknown_session" {
		t.Errorf("expected code unknown_session, got: %v", errObj)
	}
}

func TestHandleTailorSubmit_ExpiredSession(t *testing.T) {
	store := mcpserver.NewTailorSessionStore()
	input := &model.TailorInput{}
	id, err := store.Open("bundle", input, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	req := callToolRequest("tailor_submit", map[string]any{
		"session_id":    id,
		"tailored_text": "x",
		"changelog":     "[]",
	})
	result := mcpserver.HandleTailorSubmitWithStore(context.Background(), &req, store)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "session_expired" {
		t.Errorf("expected code session_expired, got: %v", errObj)
	}
}

func TestHandleTailorSubmit_AlreadyConsumed(t *testing.T) {
	store := mcpserver.NewTailorSessionStore()
	input := &model.TailorInput{}
	id, err := store.Open("bundle", input, 5*time.Second)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}

	changelog := []map[string]any{
		{"kind": "skill_add", "tier": "tier_1", "keyword": "Go"},
	}
	changelogJSON, err := json.Marshal(changelog)
	if err != nil {
		t.Fatalf("marshal changelog: %v", err)
	}

	req1 := callToolRequest("tailor_submit", map[string]any{
		"session_id":    id,
		"tailored_text": "first tailored text",
		"changelog":     string(changelogJSON),
	})
	firstResult := mcpserver.HandleTailorSubmitWithStore(context.Background(), &req1, store)
	firstText := extractText(t, firstResult)
	var firstEnv map[string]any
	if err := json.Unmarshal([]byte(firstText), &firstEnv); err != nil {
		t.Fatalf("first submit response is not JSON: %v", err)
	}
	if firstEnv["status"] != "ok" {
		t.Fatalf("first submit status = %v, want ok — full: %s", firstEnv["status"], firstText)
	}

	req2 := callToolRequest("tailor_submit", map[string]any{
		"session_id":    id,
		"tailored_text": "second tailored text",
		"changelog":     string(changelogJSON),
	})
	result := mcpserver.HandleTailorSubmitWithStore(context.Background(), &req2, store)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "session_already_consumed" {
		t.Errorf("expected code session_already_consumed, got: %v", errObj)
	}
}

func TestHandleTailorSubmit_OversizeKeyword(t *testing.T) {
	store := mcpserver.NewTailorSessionStore()
	input := &model.TailorInput{}
	id, err := store.Open("bundle", input, 5*time.Second)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}

	oversizeKeyword := strings.Repeat("a", 129)
	changelog := []map[string]any{
		{"kind": "skill_add", "tier": "tier_1", "keyword": oversizeKeyword},
	}
	changelogJSON, err := json.Marshal(changelog)
	if err != nil {
		t.Fatalf("marshal changelog: %v", err)
	}

	req := callToolRequest("tailor_submit", map[string]any{
		"session_id":    id,
		"tailored_text": "tailored content",
		"changelog":     string(changelogJSON),
	})
	result := mcpserver.HandleTailorSubmitWithStore(context.Background(), &req, store)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response is not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "invalid_changelog" {
		t.Errorf("expected code invalid_changelog, got: %v", errObj)
	}
	if errObj != nil {
		msg, _ := errObj["message"].(string)
		if !strings.Contains(msg, "Keyword") {
			t.Errorf("expected error message to contain 'Keyword', got: %q", msg)
		}
	}
}

// ── logSink: in-memory slog handler for log attribute assertions ──────────────

type logSink struct {
	mu      sync.Mutex
	records []slog.Record
}

func (l *logSink) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (l *logSink) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // slog.Handler interface mandates value receiver
	l.mu.Lock()
	l.records = append(l.records, r.Clone())
	l.mu.Unlock()
	return nil
}

func (l *logSink) WithAttrs(_ []slog.Attr) slog.Handler { return l }
func (l *logSink) WithGroup(_ string) slog.Handler      { return l }

func (l *logSink) findRecord(msg string) (*slog.Record, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.records {
		if l.records[i].Message == msg {
			return &l.records[i], true
		}
	}
	return nil, false
}

func (l *logSink) hasAttr(r *slog.Record, key, value string) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key && a.Value.String() == value {
			found = true
			return false
		}
		return true
	})
	return found
}

// ── fakeSessionStore: satisfies tailorllm.TailorStore ────────────────────────

type fakeSessionStore struct {
	openFunc func(bundle string, input *model.TailorInput, timeout time.Duration) (string, error)
	waitFunc func(ctx context.Context, id string) (model.TailorResult, error)
}

func (f *fakeSessionStore) Open(bundle string, input *model.TailorInput, timeout time.Duration) (string, error) {
	return f.openFunc(bundle, input, timeout)
}

func (f *fakeSessionStore) Wait(ctx context.Context, id string) (model.TailorResult, error) {
	return f.waitFunc(ctx, id)
}

// ── T029/T030: TestLogAttributes_TailorLifecycle ──────────────────────────────

func TestLogAttributes_TailorLifecycle(t *testing.T) {
	sink := &logSink{}
	origLogger := slog.Default()
	slog.SetDefault(slog.New(sink))
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	store := mcpserver.NewTailorSessionStore()

	// --- tailor_begin ---
	req := callToolRequest("tailor_begin", map[string]any{
		"resume_text":          "resume body",
		"accomplishments_text": "accomplishments",
	})
	result := mcpserver.HandleTailorBeginWithStore(context.Background(), &req, store, "prompt-body")
	text := extractText(t, result)
	var beginEnv map[string]any
	if err := json.Unmarshal([]byte(text), &beginEnv); err != nil || beginEnv["status"] != "ok" {
		t.Fatalf("tailor_begin failed: %s", text)
	}
	data, _ := beginEnv["data"].(map[string]any)
	sessionID, _ := data["session_id"].(string)

	r, ok := sink.findRecord("tailor_begin")
	if !ok {
		t.Error("expected tailor_begin INFO log record")
	} else if !sink.hasAttr(r, "session_id", sessionID) {
		t.Errorf("tailor_begin log missing session_id=%q", sessionID)
	}

	// --- tailor_submit ---
	changelog := []map[string]any{{"kind": "skill_add", "tier": "tier_1", "keyword": "Go"}}
	changelogJSON, _ := json.Marshal(changelog)
	req2 := callToolRequest("tailor_submit", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "tailored body",
		"changelog":     string(changelogJSON),
	})
	result2 := mcpserver.HandleTailorSubmitWithStore(context.Background(), &req2, store)
	text2 := extractText(t, result2)
	var submitEnv map[string]any
	if err := json.Unmarshal([]byte(text2), &submitEnv); err != nil || submitEnv["status"] != "ok" {
		t.Fatalf("tailor_submit failed: %s", text2)
	}

	r2, ok := sink.findRecord("tailor_submit")
	if !ok {
		t.Error("expected tailor_submit INFO log record")
	} else if !sink.hasAttr(r2, "session_id", sessionID) {
		t.Errorf("tailor_submit log missing session_id=%q", sessionID)
	}

	// --- tailor_timeout ---
	expiredStore := &fakeSessionStore{
		openFunc: func(_ string, _ *model.TailorInput, _ time.Duration) (string, error) {
			return "timeout-session", nil
		},
		waitFunc: func(_ context.Context, _ string) (model.TailorResult, error) {
			return model.TailorResult{}, errors.New("tailor session expired")
		},
	}
	llmTailor := tailorllm.New(tailorllm.Config{Timeout: 5 * time.Second, PromptBody: "p"}, expiredStore)
	_, _ = llmTailor.TailorResume(context.Background(), &model.TailorInput{ResumeText: "r"})

	r3, ok := sink.findRecord("tailor_timeout")
	if !ok {
		t.Error("expected tailor_timeout INFO log record")
	} else if !sink.hasAttr(r3, "session_id", "timeout-session") {
		t.Errorf("tailor_timeout log missing session_id")
	}

	// --- tailor_error ---
	errStore := &fakeSessionStore{
		openFunc: func(_ string, _ *model.TailorInput, _ time.Duration) (string, error) {
			return "error-session", nil
		},
		waitFunc: func(_ context.Context, _ string) (model.TailorResult, error) {
			return model.TailorResult{}, errors.New("network failure")
		},
	}
	llmTailor2 := tailorllm.New(tailorllm.Config{Timeout: 5 * time.Second, PromptBody: "p"}, errStore)
	_, _ = llmTailor2.TailorResume(context.Background(), &model.TailorInput{ResumeText: "r"})

	r4, ok := sink.findRecord("tailor_error")
	if !ok {
		t.Error("expected tailor_error INFO log record")
	} else if !sink.hasAttr(r4, "session_id", "error-session") {
		t.Errorf("tailor_error log missing session_id")
	}
}
