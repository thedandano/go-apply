package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
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
	result := mcpserver.HandleLoadJDWithConfig(context.Background(), &req, &cfg, nil)
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
	result := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &req, &cfg, &config.Config{}, nil)
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
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg, nil)
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
	result := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &req, &cfg, &config.Config{}, nil)
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
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg, nil)
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
	kwResult := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{YearsOfExperience: 5}, nil)
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
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &req, &cfg, nil)
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
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg, nil)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd response not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	req := callToolRequest("finalize", map[string]any{"session_id": sessionID})
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &req, &cfg, nil)
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
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg, nil)
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
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{}, nil)

	finReq := callToolRequest("finalize", map[string]any{
		"session_id":   sessionID,
		"cover_letter": "Dear Hiring Manager...",
	})
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &finReq, &cfg, nil)
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
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg, nil)
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
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{}, nil)

	const coverLetter = "Dear Hiring Manager, I am applying..."
	finReq := callToolRequest("finalize", map[string]any{
		"session_id":   sessionID,
		"cover_letter": coverLetter,
	})
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &finReq, &cfg, nil)
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
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg, nil)
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
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{}, nil)

	// Finalize without tailoring.
	finReq := callToolRequest("finalize", map[string]any{"session_id": sessionID})
	result := mcpserver.HandleFinalizeWithConfig(context.Background(), &finReq, &cfg, nil)
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

// TestApplicationRecord_WithChangelog_MarshalRoundTrip verifies that an ApplicationRecord
// carrying a TailorResult with Changelog marshals with tailor_result present, tailored_text
// redacted, and Changelog entries lossless after unmarshal.
func TestApplicationRecord_WithChangelog_MarshalRoundTrip(t *testing.T) {
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

// ── HandleSubmitTailoredResume helpers ───────────────────────────────────────

// countingScorer wraps a port.Scorer and fails on the first N calls, then succeeds.
type countingScorer struct {
	failFirst int
	calls     int
	inner     port.Scorer
}

var _ port.Scorer = (*countingScorer)(nil)

func (s *countingScorer) Score(in *model.ScorerInput) (model.ScoreResult, error) {
	s.calls++
	if s.calls <= s.failFirst {
		return model.ScoreResult{}, errors.New("scorer error: /tmp/some/path injection attempt")
	}
	return s.inner.Score(in)
}

// scoredSession drives load_jd then submit_keywords and returns the session ID.
func scoredSession(t *testing.T, cfg *pipeline.ApplyConfig) string {
	t.Helper()

	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Senior Go engineer. Required: go, kubernetes.",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, cfg, nil)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd response not JSON: %v — raw: %s", err, loadText)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	jdJSON := `{"title":"Go Engineer","company":"Acme","required":["go","kubernetes"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{
		"session_id": sessionID,
		"jd_json":    jdJSON,
	})
	kwResult := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, cfg, &config.Config{YearsOfExperience: 5}, nil)
	kwText := extractText(t, kwResult)
	var kwEnv map[string]any
	if err := json.Unmarshal([]byte(kwText), &kwEnv); err != nil {
		t.Fatalf("submit_keywords response not JSON: %v — raw: %s", err, kwText)
	}
	if kwEnv["status"] != "ok" {
		t.Fatalf("submit_keywords failed: %s", kwText)
	}
	return sessionID
}

// ── HandleSubmitTailoredResume tests ─────────────────────────────────────────

// TestHandleSubmitTailoredResume_HappyPath_NoChangelog verifies a clean submission
// with no changelog returns ok with previous_score and new_score.
func TestHandleSubmitTailoredResume_HappyPath_NoChangelog(t *testing.T) {
	cfg := stubApplyConfigForSession()
	sessionID := scoredSession(t, &cfg)

	req := callToolRequest("submit_tailored_resume", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "Tailored resume content with go and kubernetes experience.",
	})
	result := mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req, &cfg, &config.Config{}, nil)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	if env["next_action"] != "finalize" {
		t.Errorf("next_action = %v, want finalize", env["next_action"])
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}
	if _, ok := data["previous_score"]; !ok {
		t.Error("expected previous_score in data")
	}
	if _, ok := data["new_score"]; !ok {
		t.Error("expected new_score in data")
	}
	if data["tailored_text"] == "" {
		t.Error("expected tailored_text in data")
	}
}

// TestHandleSubmitTailoredResume_HappyPath_WithChangelog verifies a valid changelog
// round-trips through the response.
func TestHandleSubmitTailoredResume_HappyPath_WithChangelog(t *testing.T) {
	cfg := stubApplyConfigForSession()
	sessionID := scoredSession(t, &cfg)

	changelog := `[{"action":"added","target":"skill","keyword":"kubernetes","reason":"required by JD"},{"action":"rewrote","target":"bullet","keyword":"go","reason":""}]`
	req := callToolRequest("submit_tailored_resume", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "Tailored resume with kubernetes and go skills.",
		"changelog":     changelog,
	})
	result := mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req, &cfg, &config.Config{}, nil)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}
	changelogOut, _ := data["changelog"].([]any)
	if len(changelogOut) != 2 {
		t.Errorf("changelog length = %d, want 2", len(changelogOut))
	}
}

// TestHandleSubmitTailoredResume_EmptyTailoredText_ReturnsInvalidTailoredText ensures
// whitespace-only tailored_text is rejected with invalid_tailored_text code.
func TestHandleSubmitTailoredResume_EmptyTailoredText_ReturnsInvalidTailoredText(t *testing.T) {
	cfg := stubApplyConfigForSession()
	sessionID := scoredSession(t, &cfg)

	for _, emptyText := range []string{"", "   ", "\t\n"} {
		req := callToolRequest("submit_tailored_resume", map[string]any{
			"session_id":    sessionID,
			"tailored_text": emptyText,
		})
		result := mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req, &cfg, &config.Config{}, nil)
		raw := extractText(t, result)

		var env map[string]any
		if err := json.Unmarshal([]byte(raw), &env); err != nil {
			t.Fatalf("response not JSON for %q: %v", emptyText, err)
		}
		if env["status"] != "error" {
			t.Errorf("expected error status for empty text %q, got %v", emptyText, env["status"])
		}
		errObj, _ := env["error"].(map[string]any)
		if errObj == nil || errObj["code"] != "invalid_tailored_text" {
			t.Errorf("expected code invalid_tailored_text for %q, got %v", emptyText, errObj)
		}
	}
}

// TestHandleSubmitTailoredResume_InvalidAction_ReturnsInvalidChangelog verifies that
// an invalid action in the changelog produces invalid_changelog with field name + value.
func TestHandleSubmitTailoredResume_InvalidAction_ReturnsInvalidChangelog(t *testing.T) {
	cfg := stubApplyConfigForSession()
	sessionID := scoredSession(t, &cfg)

	changelog := `[{"action":"INVALID","target":"skill","keyword":"go","reason":"test"}]`
	req := callToolRequest("submit_tailored_resume", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "Resume content here.",
		"changelog":     changelog,
	})
	result := mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req, &cfg, &config.Config{}, nil)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "invalid_changelog" {
		t.Errorf("expected code invalid_changelog, got %v", errObj)
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "action") || !strings.Contains(msg, "INVALID") {
		t.Errorf("expected message to name field and value, got %q", msg)
	}
}

// TestHandleSubmitTailoredResume_InvalidTarget_ReturnsInvalidChangelog verifies that
// an invalid target in the changelog produces invalid_changelog with field name + value.
func TestHandleSubmitTailoredResume_InvalidTarget_ReturnsInvalidChangelog(t *testing.T) {
	cfg := stubApplyConfigForSession()
	sessionID := scoredSession(t, &cfg)

	changelog := `[{"action":"added","target":"BADTARGET","keyword":"go","reason":"test"}]`
	req := callToolRequest("submit_tailored_resume", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "Resume content here.",
		"changelog":     changelog,
	})
	result := mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req, &cfg, &config.Config{}, nil)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "invalid_changelog" {
		t.Errorf("expected code invalid_changelog, got %v", errObj)
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "target") || !strings.Contains(msg, "BADTARGET") {
		t.Errorf("expected message to name field and value, got %q", msg)
	}
}

// TestHandleSubmitTailoredResume_ReasonTooLong_ReturnsInvalidChangelog verifies that
// a reason exceeding 512 bytes produces invalid_changelog naming reason and length.
func TestHandleSubmitTailoredResume_ReasonTooLong_ReturnsInvalidChangelog(t *testing.T) {
	cfg := stubApplyConfigForSession()
	sessionID := scoredSession(t, &cfg)

	longReason := strings.Repeat("x", 513)
	entry := map[string]any{
		"action":  "added",
		"target":  "skill",
		"keyword": "go",
		"reason":  longReason,
	}
	changelogBytes, _ := json.Marshal([]any{entry})

	req := callToolRequest("submit_tailored_resume", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "Resume content here.",
		"changelog":     string(changelogBytes),
	})
	result := mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req, &cfg, &config.Config{}, nil)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "invalid_changelog" {
		t.Errorf("expected code invalid_changelog, got %v", errObj)
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "reason") {
		t.Errorf("expected message to name 'reason', got %q", msg)
	}
	if !strings.Contains(msg, "513") {
		t.Errorf("expected message to include observed length 513, got %q", msg)
	}
}

// TestHandleSubmitTailoredResume_InvalidState_WhenLoaded verifies that a session in
// stateLoaded is rejected with invalid_state.
func TestHandleSubmitTailoredResume_InvalidState_WhenLoaded(t *testing.T) {
	cfg := stubApplyConfigForSession()

	// Only load_jd — session stays in stateLoaded.
	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Go engineer role",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg, nil)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	req := callToolRequest("submit_tailored_resume", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "Some resume content.",
	})
	result := mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req, &cfg, &config.Config{}, nil)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "invalid_state" {
		t.Errorf("expected code invalid_state, got %v", errObj)
	}
}

// TestHandleSubmitTailoredResume_RescoreFailure_SanitizedEnvelope verifies that rescore
// errors produce a fixed sanitized message (no paths leaked), and a retry succeeds.
// scoredSession uses the standard stubScorer; the submit handler gets a failing scorer
// injected directly, then a passing scorer on the retry.
func TestHandleSubmitTailoredResume_RescoreFailure_SanitizedEnvelope(t *testing.T) {
	// Use a regular scorer for the initial scoring flow.
	initialCfg := stubApplyConfigForSession()
	sessionID := scoredSession(t, &initialCfg)

	// Inject a scorer that always fails for the first submit_tailored_resume call.
	failingCfg := pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		Scorer:    &countingScorer{failFirst: 1, inner: &stubScorer{}},
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubApplicationRepository{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}

	req1 := callToolRequest("submit_tailored_resume", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "Tailored resume content.",
	})
	result1 := mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req1, &failingCfg, &config.Config{}, nil)
	text1 := extractText(t, result1)

	var env1 map[string]any
	if err := json.Unmarshal([]byte(text1), &env1); err != nil {
		t.Fatalf("response not JSON: %v — raw: %s", err, text1)
	}
	if env1["status"] != "error" {
		t.Errorf("expected error status on rescore failure, got %v", env1["status"])
	}
	errObj, _ := env1["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "rescore_failed" {
		t.Errorf("expected code rescore_failed, got %v", errObj)
	}
	msg, _ := errObj["message"].(string)
	if msg != "rescore failed; see server logs for details" {
		t.Errorf("expected fixed sanitized message, got %q", msg)
	}
	if strings.Contains(msg, "/") {
		t.Errorf("envelope message must not contain path separator '/': %q", msg)
	}

	// Verify the session was left in stateTailored by the failure path.
	if state, ok := mcpserver.GetSessionStateForTest(sessionID); !ok || state != "tailored" {
		t.Fatalf("expected session state %q after rescore failure, got %q (ok=%v)", "tailored", state, ok)
	}

	// Retry: session is now in stateTailored; inject a passing scorer.
	passingCfg := stubApplyConfigForSession()
	req2 := callToolRequest("submit_tailored_resume", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "Tailored resume content on retry.",
	})
	result2 := mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req2, &passingCfg, &config.Config{}, nil)
	text2 := extractText(t, result2)

	var env2 map[string]any
	if err := json.Unmarshal([]byte(text2), &env2); err != nil {
		t.Fatalf("retry response not JSON: %v — raw: %s", err, text2)
	}
	if env2["status"] != "ok" {
		t.Errorf("expected ok on retry, got %v — full: %s", env2["status"], text2)
	}
}

// TestHandleSubmitTailoredResume_VerboseGating_ResultPayload verifies that the debug
// "mcp tool result" log carries the full result payload only when verbose is on.
// The tailored_text is intentionally large (>2048 bytes) so that PayloadAttr's
// truncation actually fires under verbose=false.
func TestHandleSubmitTailoredResume_VerboseGating_ResultPayload(t *testing.T) {
	cfg := stubApplyConfigForSession()
	appCfg := &config.Config{}

	// Unique sentinel in the middle so it falls in the dropped zone after truncation.
	// PayloadAttr truncates at 2048 bytes (keeping first and last 1024). The sentinel is
	// placed at ~50% of the JSON payload so it is omitted when verbose=false.
	const uniqueSentinel = "UNIQUE_SENTINEL_MID"
	half := strings.Repeat("Resume line content here. ", 60) // ~1560 bytes each side
	longTailored := half + uniqueSentinel + half

	for _, verbose := range []bool{false, true} {
		name := "verbose_off"
		if verbose {
			name = "verbose_on"
		}
		t.Run(name, func(t *testing.T) {
			logger.SetVerbose(verbose)
			t.Cleanup(func() { logger.SetVerbose(false) })

			var buf bytes.Buffer
			h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
			slog.SetDefault(slog.New(h))
			t.Cleanup(func() { slog.SetDefault(slog.Default()) })

			sessionID := scoredSession(t, &cfg)

			req := callToolRequest("submit_tailored_resume", map[string]any{
				"session_id":    sessionID,
				"tailored_text": longTailored,
			})
			_ = mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req, &cfg, appCfg, nil)

			logOutput := buf.String()
			lines := strings.Split(strings.TrimSpace(logOutput), "\n")

			// Find the "mcp tool result" debug line for submit_tailored_resume.
			var toolResultLine string
			for _, line := range lines {
				if strings.Contains(line, "mcp tool result") && strings.Contains(line, "submit_tailored_resume") {
					toolResultLine = line
					break
				}
			}
			if toolResultLine == "" {
				t.Fatal("expected 'mcp tool result' debug log line for submit_tailored_resume, not found")
			}

			if verbose {
				// When verbose, the full result payload key should be present with content.
				if !strings.Contains(toolResultLine, `"result"`) {
					t.Error("verbose=true: expected 'result' key in debug log line")
				}
			} else {
				// When not verbose, PayloadAttr truncates the payload: the unique sentinel
				// in the middle of longTailored must be absent (it falls in the dropped zone),
				// and the truncation marker must be present, proving the full text was not emitted.
				if strings.Contains(toolResultLine, uniqueSentinel) {
					t.Error("verbose=false: mid-payload sentinel found in log — PayloadAttr must truncate large payloads")
				}
				if !strings.Contains(toolResultLine, "bytes omitted") {
					t.Error("verbose=false: expected truncation marker 'bytes omitted' in log result field")
				}
			}
		})
	}
}

// TestHandleSubmitTailoredResume_InfoLog_FourAttributes verifies the "tailor submission
// complete" info log emits exactly the 4 required numeric attributes.
func TestHandleSubmitTailoredResume_InfoLog_FourAttributes(t *testing.T) {
	cfg := stubApplyConfigForSession()
	appCfg := &config.Config{}

	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(slog.Default()) })

	sessionID := scoredSession(t, &cfg)

	tailored := "Line one\nLine two\nLine three"
	req := callToolRequest("submit_tailored_resume", map[string]any{
		"session_id":    sessionID,
		"tailored_text": tailored,
	})
	_ = mcpserver.HandleSubmitTailoredResumeWithConfig(context.Background(), &req, &cfg, appCfg, nil)

	logOutput := buf.String()
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")

	var infoLine string
	for _, line := range lines {
		if strings.Contains(line, "tailor submission complete") {
			infoLine = line
			break
		}
	}
	if infoLine == "" {
		t.Fatalf("expected 'tailor submission complete' info log, not found in:\n%s", logOutput)
	}

	var attrs map[string]any
	if err := json.Unmarshal([]byte(infoLine), &attrs); err != nil {
		t.Fatalf("info log line not JSON: %v — raw: %s", err, infoLine)
	}

	for _, key := range []string{"tailored_text_bytes", "tailored_text_lines", "previous_score", "new_score"} {
		if _, ok := attrs[key]; !ok {
			t.Errorf("expected attribute %q in info log, not found — attrs: %v", key, attrs)
		}
	}
	if attrs["tailored_text_bytes"] != float64(len(tailored)) {
		t.Errorf("tailored_text_bytes = %v, want %d", attrs["tailored_text_bytes"], len(tailored))
	}
	if attrs["tailored_text_lines"] != float64(3) {
		t.Errorf("tailored_text_lines = %v, want 3", attrs["tailored_text_lines"])
	}
}
