package mcpserver_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// stubApplyConfigWithPDFScoring returns a config that supports PDF scoring:
// it has sections, a PDF renderer, extractor, and survival differ.
func stubApplyConfigWithPDFScoring() pipeline.ApplyConfig {
	cfg := stubApplyConfigWithSkillsLoader()
	cfg.PDFRenderer = &stubPDFRenderer{failRender: false}
	cfg.Extractor = &stubExtractor{failExtract: false}
	cfg.SurvivalDiffer = &stubSurvivalDiffer{}
	return cfg
}

// ── T011 failing tests for T0/T1/T2 wiring ───────────────────────────────────

// T011-T0: when extraction fails for any resume, the whole call returns an error.
// This test will initially be red until T012 is implemented.
func TestHandleSubmitKeywords_ExtractionFails_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader()
	cfg.PDFRenderer = &stubPDFRenderer{failRender: false}
	cfg.Extractor = &stubExtractor{failExtract: true} // always fails
	cfg.SurvivalDiffer = &stubSurvivalDiffer{}

	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[]}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	result := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v — raw: %s", err, text)
	}
	// Hard error — no partial results.
	if env["status"] != "error" {
		t.Errorf("status = %v, want error when extraction fails for a resume", env["status"])
	}
}

// T011-T1: response JSON contains "scoring_method": "pdf_extracted".
// This test will initially be red until T013 is implemented.
func TestHandleSubmitTailorT1_ResponseContainsScoringMethod(t *testing.T) {
	cfg := stubApplyConfigWithPDFScoring()

	// Full flow: load → keywords → T1.
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer. Skills: go."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[]}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"skills","op":"add","value":"EKS"}]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{})
	text := extractText(t, result)

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
	if data["scoring_method"] != mcpserver.ScoringMethodPDFExtracted {
		t.Errorf("data.scoring_method = %v, want %q", data["scoring_method"], mcpserver.ScoringMethodPDFExtracted)
	}
}

// T011-T2: response JSON contains "scoring_method": "pdf_extracted".
// This test will initially be red until T013 is implemented.
func TestHandleSubmitTailorT2_ResponseContainsScoringMethod(t *testing.T) {
	// Use experience sections for T2.
	cfg := pipeline.ApplyConfig{
		Fetcher:        &stubJDFetcher{},
		Scorer:         &stubScorer{},
		Resumes:        &stubResumeRepoWithExperienceSections{},
		Loader:         &stubDocumentLoader{},
		AppRepo:        &stubApplicationRepository{},
		Defaults:       &config.AppDefaults{},
		PDFRenderer:    &stubPDFRenderer{failRender: false},
		Extractor:      &stubExtractor{failExtract: false},
		SurvivalDiffer: &stubSurvivalDiffer{},
	}

	// load → keywords → T2.
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer. Skills: go."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[]}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	// T2 requires T1 to run first.
	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"skills","op":"add","value":"Kubernetes"}]`,
	})
	mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{})

	t2Req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"experience","op":"replace","target":"exp-0-b0","value":"Built Go microservices on Kubernetes"}]`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &t2Req, &cfg, &config.Config{})
	text := extractText(t, result)

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
	if data["scoring_method"] != mcpserver.ScoringMethodPDFExtracted {
		t.Errorf("data.scoring_method = %v, want %q", data["scoring_method"], mcpserver.ScoringMethodPDFExtracted)
	}
}
