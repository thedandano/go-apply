package mcpserver_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// stubAugmenterWithSuggestions returns a populated TailorSuggestions for testing.
type stubAugmenterWithSuggestions struct {
	suggestions model.TailorSuggestions
	err         error
}

func (s *stubAugmenterWithSuggestions) AugmentResumeText(_ context.Context, input model.AugmentInput) (string, *model.ReferenceData, error) {
	return input.ResumeText, input.RefData, nil
}

func (s *stubAugmenterWithSuggestions) SuggestForKeywords(_ context.Context, _ []string) (model.TailorSuggestions, error) {
	return s.suggestions, s.err
}

// stubApplyConfigWithAugmenter builds an ApplyConfig using the given Augmenter.
func stubApplyConfigWithAugmenter(aug interface {
	AugmentResumeText(context.Context, model.AugmentInput) (string, *model.ReferenceData, error)
	SuggestForKeywords(context.Context, []string) (model.TailorSuggestions, error)
}) pipeline.ApplyConfig {
	return pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       &stubLLMClient{},
		Scorer:    &stubScorer{},
		CLGen:     nil,
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubApplicationRepository{},
		Augment:   aug,
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}
}

// scoredSession creates a load_jd + submit_keywords session and returns the session ID.
func scoredSession(t *testing.T, cfg *pipeline.ApplyConfig) string {
	t.Helper()

	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Senior Go engineer. Required: go, kubernetes.",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, cfg)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd response not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	jdJSON := `{"title":"Go Engineer","company":"Acme","required":["go","kubernetes"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{
		"session_id": sessionID,
		"jd_json":    jdJSON,
	})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, cfg, &config.Config{})

	return sessionID
}

// ── HandleSuggestTailoring tests ─────────────────────────────────────────────

func TestHandleSuggestTailoring_SessionNotFound_ReturnsError(t *testing.T) {
	req := callToolRequest("suggest_tailoring", map[string]any{
		"session_id": "nonexistent-session-id",
	})
	result := mcpserver.HandleSuggestTailoringWithConfig(context.Background(), &req, nil)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "session_not_found" {
		t.Errorf("expected code session_not_found, got %v", errObj)
	}
}

func TestHandleSuggestTailoring_NotYetScored_ReturnsInvalidState(t *testing.T) {
	cfg := stubApplyConfigForSession()

	// load_jd only — session is stateLoaded, not stateScored
	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Go engineer role",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg)
	loadText := extractText(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	req := callToolRequest("suggest_tailoring", map[string]any{
		"session_id": sessionID,
	})
	result := mcpserver.HandleSuggestTailoringWithConfig(context.Background(), &req, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
	if !strings.Contains(text, "invalid_state") {
		t.Errorf("expected invalid_state code, got: %s", text)
	}
}

func TestHandleSuggestTailoring_NilAugmenter_ReturnsNone(t *testing.T) {
	cfg := stubApplyConfigForSession()
	// Override augmenter with nil to test the no-augmenter path.
	cfg.Augment = nil

	sessionID := scoredSession(t, &cfg)

	req := callToolRequest("suggest_tailoring", map[string]any{
		"session_id": sessionID,
	})
	result := mcpserver.HandleSuggestTailoringWithConfig(context.Background(), &req, &cfg)
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
	if data["retrieval_mode"] != "none" {
		t.Errorf("retrieval_mode = %v, want none", data["retrieval_mode"])
	}
	// With nil augmenter, keywords are listed but all have empty matches.
	required, _ := data["required"].([]any)
	for _, item := range required {
		entry, _ := item.(map[string]any)
		matches, _ := entry["matches"].([]any)
		if len(matches) != 0 {
			t.Errorf("keyword %v: expected empty matches with nil augmenter, got %v", entry["keyword"], matches)
		}
	}
	preferred, _ := data["preferred"].([]any)
	for _, item := range preferred {
		entry, _ := item.(map[string]any)
		matches, _ := entry["matches"].([]any)
		if len(matches) != 0 {
			t.Errorf("keyword %v: expected empty matches with nil augmenter, got %v", entry["keyword"], matches)
		}
	}
}

func TestHandleSuggestTailoring_WithSuggestions_ReturnsModeAndMatches(t *testing.T) {
	suggestions := model.TailorSuggestions{
		"go": {
			{Keyword: "go", SourceDoc: "profile.md", Text: "Expert in Go concurrency", Similarity: 0.92},
		},
		"kubernetes": {
			{Keyword: "kubernetes", SourceDoc: "skills.md", Text: "Deployed K8s clusters", Similarity: 0.0},
		},
	}
	aug := &stubAugmenterWithSuggestions{suggestions: suggestions}
	cfg := stubApplyConfigWithAugmenter(aug)

	sessionID := scoredSession(t, &cfg)

	req := callToolRequest("suggest_tailoring", map[string]any{
		"session_id": sessionID,
	})
	result := mcpserver.HandleSuggestTailoringWithConfig(context.Background(), &req, &cfg)
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

	// "go" has Similarity > 0 → retrieval_mode should be "vector"
	if data["retrieval_mode"] != "vector" {
		t.Errorf("retrieval_mode = %v, want vector", data["retrieval_mode"])
	}

	required, _ := data["required"].([]any)
	if len(required) == 0 {
		t.Fatal("expected non-empty required")
	}

	// Find the "go" entry and verify its match.
	var goEntry map[string]any
	for _, item := range required {
		entry, _ := item.(map[string]any)
		if entry["keyword"] == "go" {
			goEntry = entry
			break
		}
	}
	if goEntry == nil {
		t.Fatalf("expected keyword 'go' in required, got: %v", required)
	}
	matches, _ := goEntry["matches"].([]any)
	if len(matches) == 0 {
		t.Fatal("expected matches for keyword 'go'")
	}
	match, _ := matches[0].(map[string]any)
	if match["source"] != "profile.md" {
		t.Errorf("match source = %v, want profile.md", match["source"])
	}
	if match["text"] != "Expert in Go concurrency" {
		t.Errorf("match text = %v, want 'Expert in Go concurrency'", match["text"])
	}
}
