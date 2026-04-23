package mcpserver_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// capturingAppRepo is a test double that records the last record passed to Put.
// Used to assert what HandleFinalizeWithConfig persists to the application repository.
type capturingAppRepo struct {
	last *model.ApplicationRecord
}

var _ port.ApplicationRepository = (*capturingAppRepo)(nil)

func (c *capturingAppRepo) Get(_ string) (*model.ApplicationRecord, bool, error) {
	return nil, false, nil
}
func (c *capturingAppRepo) Put(rec *model.ApplicationRecord) error {
	clone := *rec
	c.last = &clone
	return nil
}
func (c *capturingAppRepo) Update(_ *model.ApplicationRecord) error   { return nil }
func (c *capturingAppRepo) List() ([]*model.ApplicationRecord, error) { return nil, nil }

// stubApplyConfigForSession returns an ApplyConfig with all stubs and no Presenter set.
// The handlers set the presenter internally.
func stubApplyConfigForSession() pipeline.ApplyConfig {
	return pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		Scorer:    &stubScorer{},
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

// ── finalize persistence tests ────────────────────────────────────────────────

// TestHandleFinalizeWithConfig_NeverTailored_NoTailorResultPersisted verifies that when a
// session goes through load_jd → submit_keywords → finalize without any tailoring, the
// persisted ApplicationRecord contains no tailor_result key.
func TestHandleFinalizeWithConfig_NeverTailored_NoTailorResultPersisted(t *testing.T) {
	capturing := &capturingAppRepo{}
	cfg := pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		Scorer:    &stubScorer{},
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   capturing,
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}

	// Load JD with a URL so the persistence path in finalize is triggered.
	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_url": "https://example.com/job/never-tailored",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	// Score resumes.
	kwReq := callToolRequest("submit_keywords", map[string]any{
		"session_id": sessionID,
		"jd_json":    `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[]}`,
	})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	// Finalize without tailoring.
	finReq := callToolRequest("finalize", map[string]any{"session_id": sessionID})
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &finReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("finalize not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Fatalf("finalize status = %v, want ok — full: %s", env["status"], text)
	}

	// The capturing repo must have received a record with no TailorResult.
	if capturing.last == nil {
		t.Fatal("expected AppRepo.Put to be called, but it was not")
	}
	persisted, err := json.Marshal(capturing.last)
	if err != nil {
		t.Fatalf("marshal persisted record: %v", err)
	}
	if strings.Contains(string(persisted), `"tailor_result"`) {
		t.Errorf("persisted record must not contain tailor_result for a never-tailored session; got: %s", persisted)
	}
}

// TestHandleFinalizeWithConfig_TailoredSession_TailorResultAndChangelogPersisted verifies
// that an ApplicationRecord with TailorResult and a Changelog marshals correctly:
// tailor_result key is present, changelog entries are lossless, and tailored_text is redacted.
// This tests the persistence artifact directly because injecting Changelog into session state
// requires the Unit 3 submit_tailor handler (not yet implemented).
func TestHandleFinalizeWithConfig_TailoredSession_TailorResultAndChangelogPersisted(t *testing.T) {
	changelog := []model.ChangelogEntry{
		{Action: "added", Target: "skill", Keyword: "kubernetes", Reason: "required by JD"},
		{Action: "rewrote", Target: "bullet", Keyword: "go", Reason: ""},
		{Action: "skipped", Target: "summary", Keyword: "docker"},
	}
	rec := &model.ApplicationRecord{
		URL: "https://example.com/job/tailored",
		TailorResult: &model.TailorResult{
			ResumeLabel:  "main",
			TailoredText: "full tailored resume body — must be redacted on disk",
			Changelog:    changelog,
		},
	}

	persisted, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal ApplicationRecord: %v", err)
	}

	// tailor_result must be present.
	if !strings.Contains(string(persisted), `"tailor_result"`) {
		t.Errorf("tailor_result missing from persisted record; got: %s", persisted)
	}

	// tailored_text must be redacted (absent from disk artifact).
	if strings.Contains(string(persisted), `"tailored_text"`) {
		t.Errorf("tailored_text must be redacted from persisted record; got: %s", persisted)
	}

	// Unmarshal and verify changelog round-trips losslessly.
	var decoded model.ApplicationRecord
	if err := json.Unmarshal(persisted, &decoded); err != nil {
		t.Fatalf("unmarshal persisted record: %v", err)
	}
	if decoded.TailorResult == nil {
		t.Fatal("TailorResult is nil after unmarshal")
	}
	if len(decoded.TailorResult.Changelog) != len(changelog) {
		t.Fatalf("Changelog length = %d, want %d", len(decoded.TailorResult.Changelog), len(changelog))
	}
	for i, want := range changelog {
		got := decoded.TailorResult.Changelog[i]
		if got.Action != want.Action || got.Target != want.Target || got.Keyword != want.Keyword || got.Reason != want.Reason {
			t.Errorf("Changelog[%d] = %+v, want %+v", i, got, want)
		}
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
