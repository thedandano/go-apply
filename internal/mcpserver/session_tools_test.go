package mcpserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	extractPkg "github.com/thedandano/go-apply/internal/service/extract"
	pdfrender "github.com/thedandano/go-apply/internal/service/pdfrender"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/survival"
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

// stubDocumentLoaderWithSkills returns a resume that has a ## Skills section.
type stubDocumentLoaderWithSkills struct{}

func (s *stubDocumentLoaderWithSkills) Load(_ string) (string, error) {
	return "# Experience\n- Built distributed systems\n\n## Skills\nCloud: AWS, GCP\nLanguages: Go, Python\n\n# Education\nBSc CS", nil
}
func (s *stubDocumentLoaderWithSkills) Supports(_ string) bool { return true }

// stubResumeRepoWithSkillsSections extends stubResumeRepo to return sections with a skills flat field.
type stubResumeRepoWithSkillsSections struct {
	stubResumeRepo
}

func (s *stubResumeRepoWithSkillsSections) LoadSections(_ string) (model.SectionMap, error) {
	return model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Skills: &model.SkillsSection{
			Kind: model.SkillsKindFlat,
			Flat: "Cloud: AWS, GCP\nLanguages: Go, Python",
		},
	}, nil
}

func stubApplyConfigWithSkillsLoader() pipeline.ApplyConfig {
	cfg := stubApplyConfigForSession()
	cfg.Loader = &stubDocumentLoaderWithSkills{}
	cfg.Resumes = &stubResumeRepoWithSkillsSections{}
	return cfg
}

// stubResumeRepoWithExperienceSections returns sections with both skills and experience entries.
type stubResumeRepoWithExperienceSections struct {
	stubResumeRepo
}

func (s *stubResumeRepoWithExperienceSections) LoadSections(_ string) (model.SectionMap, error) {
	return model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Skills: &model.SkillsSection{
			Kind: model.SkillsKindFlat,
			Flat: "Go, Python",
		},
		Experience: []model.ExperienceEntry{
			{
				Company:   "Acme Corp",
				Role:      "Engineer",
				StartDate: "2020-01",
				Bullets:   []string{"Built distributed systems in Go", "Led migration to Kubernetes"},
			},
		},
	}, nil
}

func stubApplyConfigWithExperienceSections() pipeline.ApplyConfig {
	cfg := stubApplyConfigForSession()
	cfg.Resumes = &stubResumeRepoWithExperienceSections{}
	return cfg
}

// T010: submit_keywords includes skills_section field when best resume has a Skills header.
func TestHandleSubmitKeywordsWithConfig_ReturnsSkillsSection(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader()

	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	kwResult := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{YearsOfExperience: 5})
	kwText := extractText(t, kwResult)

	var env map[string]any
	if err := json.Unmarshal([]byte(kwText), &env); err != nil {
		t.Fatalf("submit_keywords not JSON: %v — raw: %s", err, kwText)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — full: %s", env["status"], kwText)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}
	skillsSection, ok := data["skills_section"].(string)
	if !ok || skillsSection == "" {
		t.Errorf("expected non-empty skills_section in data, got: %v", data["skills_section"])
	}
	if strings.Contains(skillsSection, "## Skills") {
		t.Error("skills_section must not include the header line")
	}
	if !strings.Contains(skillsSection, "Go, Python") {
		t.Errorf("skills_section body missing expected content, got: %q", skillsSection)
	}
}

// T010: skills_section is always present in the envelope (empty string when no sections sidecar).
func TestHandleSubmitKeywordsWithConfig_NoSkillsSection_FieldAlwaysPresent(t *testing.T) {
	cfg := stubApplyConfigForSession() // stubResumeRepo.LoadSections returns ErrSectionsMissing

	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	kwResult := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{YearsOfExperience: 5})
	kwText := extractText(t, kwResult)

	var env map[string]any
	if err := json.Unmarshal([]byte(kwText), &env); err != nil {
		t.Fatalf("submit_keywords not JSON: %v — raw: %s", err, kwText)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — full: %s", env["status"], kwText)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}
	// skills_section is always present now (no omitempty).
	if _, present := data["skills_section"]; !present {
		t.Error("skills_section must always be present in the envelope")
	}
	if ss := data["skills_section"].(string); ss != "" {
		t.Errorf("skills_section must be empty when no sections sidecar, got: %q", ss)
	}
	// skills_section_found must be explicit false.
	if found, _ := data["skills_section_found"].(bool); found {
		t.Error("skills_section_found must be false when no sections sidecar")
	}
	// sections must be absent (omitempty) when no sidecar.
	if _, present := data["sections"]; present {
		t.Error("sections must be absent when no sidecar exists")
	}
}

// US3: submit_keywords includes sections + skills_section_found when sidecar exists.
func TestHandleSubmitKeywordsWithConfig_Sections_PresentWhenSidecarExists(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader() // uses stubResumeRepoWithSkillsSections

	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[]}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	kwResult := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{YearsOfExperience: 5})
	kwText := extractText(t, kwResult)

	var env map[string]any
	if err := json.Unmarshal([]byte(kwText), &env); err != nil {
		t.Fatalf("submit_keywords not JSON: %v", err)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — full: %s", env["status"], kwText)
	}
	data, _ := env["data"].(map[string]any)

	if found, _ := data["skills_section_found"].(bool); !found {
		t.Error("skills_section_found must be true when sidecar has skills")
	}
	if sections, present := data["sections"]; !present || sections == nil {
		t.Error("sections must be present and non-nil when sidecar exists")
	}
}

// US3: preview_ats_extraction returns constructed text for the best resume in the session.
// Uses stubApplyConfigWithSkillsLoader so LoadSections succeeds, and injects stub
// PDFRenderer + Extractor so the test does not require a real pdftotext installation.
func TestHandlePreviewATSExtraction_ReturnsConstructedText(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader()
	cfg.PDFRenderer = &stubPDFRenderer{failRender: false}
	cfg.Extractor = &stubExtractor{failExtract: false}
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
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{YearsOfExperience: 5})

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("preview_ats_extraction not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}
	if data["constructed_text"] == nil || data["constructed_text"] == "" {
		t.Errorf("constructed_text must be non-empty, got: %v", data["constructed_text"])
	}
	if data["label"] == nil || data["label"] == "" {
		t.Errorf("label must be non-empty, got: %v", data["label"])
	}
}

// US3: preview_ats_extraction returns error when session not found.
func TestHandlePreviewATSExtraction_SessionNotFound(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("preview_ats_extraction", map[string]any{"session_id": "nonexistent"})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &req, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
}

// US3: preview_ats_extraction returns error when session_id missing.
func TestHandlePreviewATSExtraction_MissingSessionID(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("preview_ats_extraction", map[string]any{})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &req, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error", env["status"])
	}
}

// T021 (SC-002): preview_ats_extraction must include Tier 4 section content when the
// resume SectionMap contains Tier 4 sections.
type stubResumeRepoWithTier4Sections struct {
	stubResumeRepo
}

func (s *stubResumeRepoWithTier4Sections) LoadSections(_ string) (model.SectionMap, error) {
	return model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Alice"},
		Experience: []model.ExperienceEntry{
			{Company: "Acme", Role: "Engineer", StartDate: "2020-01", Bullets: []string{"Built systems"}},
		},
		Languages:  []model.LanguageEntry{{Name: "Go", Proficiency: "Fluent"}},
		Speaking:   []model.SpeakingEntry{{Title: "GopherCon", Event: "Conf", Date: "2023"}},
		OpenSource: []model.OpenSourceEntry{{Project: "go-apply", Role: "Author"}},
		Patents:    []model.PatentEntry{{Title: "Fast Algorithm", Number: "US123"}},
		Interests:  []model.InterestEntry{{Name: "Distributed systems"}},
		References: []model.ReferenceEntry{{Name: "Available upon request"}},
	}, nil
}

func stubApplyConfigWithTier4Sections() pipeline.ApplyConfig {
	cfg := stubApplyConfigForSession()
	cfg.Resumes = &stubResumeRepoWithTier4Sections{}
	return cfg
}

func TestHandlePreviewATSExtraction_Tier4SectionInConstructedText(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed — skipping Tier 4 content check")
	}
	cfg := stubApplyConfigWithTier4Sections()
	cfg.PDFRenderer = pdfrender.New()
	cfg.Extractor = extractPkg.New()
	cfg.SurvivalDiffer = survival.New()

	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[]}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{YearsOfExperience: 5})

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("preview_ats_extraction not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	constructedText, _ := data["constructed_text"].(string)
	for _, heading := range []string{"LANGUAGES", "SPEAKING ENGAGEMENTS", "OPEN SOURCE", "PATENTS", "INTERESTS", "REFERENCES"} {
		if !strings.Contains(constructedText, heading) {
			t.Errorf("constructed_text missing Tier 4 heading %q; got:\n%s", heading, constructedText)
		}
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
		"edits":      `[{"section":"skills","op":"add","value":"K8s"}]`,
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
	// load_jd only — state stays stateLoaded (not stateScored)
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Go engineer role"})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	_ = json.Unmarshal([]byte(loadText), &loadEnv)
	sessionID, _ := loadEnv["session_id"].(string)

	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"skills","op":"add","value":"Rust"}]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_state") {
		t.Errorf("expected invalid_state error, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_HappyPath_ReturnsEditsApplied(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader() // has sections sidecar with flat skills

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

	// submit_tailor_t1 with structured edits
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
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil || data["new_score"] == nil {
		t.Errorf("expected new_score in data, got: %s", text)
	}
	if data["previous_score"] == nil {
		t.Errorf("expected previous_score in data, got: %s", text)
	}
	if _, ok := data["edits_applied"]; !ok {
		t.Errorf("expected edits_applied key in data, got: %s", text)
	}
	if _, ok := data["edits_rejected"]; !ok {
		t.Errorf("expected edits_rejected key in data, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_MissingEdits_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": "any-id",
		// edits absent
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "missing_edits") {
		t.Errorf("expected missing_edits error, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_InvalidEditsJSON_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": "any-id",
		"edits":      `not-valid-json`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_edits") {
		t.Errorf("expected invalid_edits error, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_EmptyEdits_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": "any-id",
		"edits":      `[]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "empty_edits") {
		t.Errorf("expected empty_edits error, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_WrongSection_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": "any-id",
		"edits":      `[{"section":"experience","op":"add","value":"x"}]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_section") {
		t.Errorf("expected invalid_section error, got: %s", text)
	}
}

func TestHandleSubmitTailorT1_TooManyEdits_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	cfg.Defaults = &config.AppDefaults{
		Tailor: config.TailorDefaults{MaxTier1SkillRewrites: 2},
	}

	// Build a valid scored session.
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Go engineer role."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg))
	var loadEnv map[string]any
	_ = json.Unmarshal([]byte(loadText), &loadEnv)
	sessionID, _ := loadEnv["session_id"].(string)
	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"]}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	// 3 edits exceeds cap of 2.
	t1Req := callToolRequest("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"edits": `[{"section":"skills","op":"add","value":"A"},` +
			`{"section":"skills","op":"add","value":"B"},` +
			`{"section":"skills","op":"add","value":"C"}]`,
	})
	result := mcpserver.HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "too_many_edits") {
		t.Errorf("expected too_many_edits error, got: %s", text)
	}
}

// ── HandleSubmitTailorT2 tests ────────────────────────────────────────────────

func TestHandleSubmitTailorT2_SessionNotFound_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": "no-such-session",
		"edits":      `[{"section":"experience","op":"replace","target":"exp-0-b0","value":"new bullet"}]`,
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
		"session_id": sessionID,
		"edits":      `[{"section":"experience","op":"replace","target":"exp-0-b0","value":"new bullet"}]`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_state") {
		t.Errorf("expected invalid_state error, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_MissingEdits_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": "any-id",
		// edits missing
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "missing_edits") {
		t.Errorf("expected missing_edits error, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_InvalidEditsJSON_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": "any-id",
		"edits":      `not-valid-json`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_edits") {
		t.Errorf("expected invalid_edits error, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_EmptyEdits_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": "any-id",
		"edits":      `[]`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "empty_edits") {
		t.Errorf("expected empty_edits error, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_WrongSection_ReturnsError(t *testing.T) {
	cfg := stubApplyConfigForSession()
	req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": "any-id",
		"edits":      `[{"section":"skills","op":"add","value":"x"}]`,
	})
	result := mcpserver.HandleSubmitTailorT2WithConfig(context.Background(), &req, &cfg, &config.Config{})
	text := extractText(t, result)
	if !strings.Contains(text, "invalid_section") {
		t.Errorf("expected invalid_section error, got: %s", text)
	}
}

func TestHandleSubmitTailorT2_HappyPath_ReturnsNewScore(t *testing.T) {
	cfg := stubApplyConfigWithExperienceSections() // has sections with experience bullets

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
	t2Req := callToolRequest("submit_tailor_t2", map[string]any{
		"session_id": sessionID,
		"edits":      `[{"section":"experience","op":"replace","target":"exp-0-b0","value":"Built distributed systems in Go and Kubernetes"}]`,
	})
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
	if data == nil || data["new_score"] == nil {
		t.Errorf("expected new_score in data, got: %s", text)
	}
	if _, ok := data["edits_applied"]; !ok {
		t.Errorf("expected edits_applied key in data, got: %s", text)
	}
	if _, ok := data["edits_rejected"]; !ok {
		t.Errorf("expected edits_rejected key in data, got: %s", text)
	}
}

// ── T007: preview_ats_extraction hard-fail tests (RED STATE until T012) ───────
//
// T012 must:
//   - Add PDFRenderer port.PDFRenderer to pipeline.ApplyConfig (field: cfg.PDFRenderer)
//   - Add Extractor port.Extractor to pipeline.ApplyConfig (field: cfg.Extractor)
//   - Remove the silent fallback to rawText when LoadSections fails → return stageErrorEnvelope "no_sections_data"
//   - Remove the silent fallback when Render fails → return stageErrorEnvelope "render_failed"
//   - Remove the silent fallback when Extract fails → return stageErrorEnvelope "extract_failed"
//
// NOTE: TestHandlePreviewATSExtraction_ReturnsConstructedText (line 393) uses
// stubApplyConfigForSession() whose LoadSections returns ErrSectionsMissing.
// After T012 removes the fallback, that test will break — T012 must migrate it
// to a sections-bearing stub (e.g., stubApplyConfigWithSkillsLoader).

// stubPDFRenderer is an injectable PDF renderer for tests.
// successPDF controls whether RenderPDF succeeds (returns fake PDF bytes) or fails.
type stubPDFRenderer struct {
	failRender bool
}

func (s *stubPDFRenderer) RenderPDF(_ *model.SectionMap) ([]byte, error) {
	if s.failRender {
		return nil, fmt.Errorf("render error")
	}
	return []byte("%PDF-1.4 fake"), nil
}

// stubExtractor is an injectable text extractor for tests.
// failExtract controls whether Extract succeeds or fails.
type stubExtractor struct {
	failExtract bool
}

func (s *stubExtractor) Extract(_ context.Context, _ []byte) (string, error) {
	if s.failExtract {
		return "", fmt.Errorf("extract error")
	}
	return "extracted text", nil
}

// stubSurvivalDiffer is an injectable survival differ for tests.
type stubSurvivalDiffer struct{}

func (s *stubSurvivalDiffer) Diff(keywords []string, _ string) model.KeywordSurvival {
	return model.KeywordSurvival{
		Dropped:         []string{},
		Matched:         keywords,
		TotalJDKeywords: len(keywords),
	}
}

// stubScorerWithKeywords is a scorer that echoes JD keywords into KeywordResult so
// keyword-survival tests can assert on non-empty keyword lists.
type stubScorerWithKeywords struct{}

func (s *stubScorerWithKeywords) Score(input *model.ScorerInput) (model.ScoreResult, error) {
	return model.ScoreResult{
		Breakdown: model.ScoreBreakdown{
			KeywordMatch: 0.9, ExperienceFit: 0.9, ImpactEvidence: 0.9, ATSFormat: 0.9, Readability: 0.9,
		},
		Keywords: model.KeywordResult{
			ReqUnmatched:  input.JD.Required,
			PrefUnmatched: input.JD.Preferred,
		},
	}, nil
}

// buildScoredSession creates a scored session via load_jd + submit_keywords and returns the session ID.
func buildScoredSession(t *testing.T, cfg *pipeline.ApplyConfig) string {
	t.Helper()
	loadReq := callToolRequest("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadText := extractText(t, mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, cfg))
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v — raw: %s", err, loadText)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[]}`
	kwReq := callToolRequest("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, cfg, &config.Config{YearsOfExperience: 5})
	return sessionID
}

// TestHandlePreviewATSExtraction_NoSectionsSidecar_HardFails asserts that when
// LoadSections returns an error the handler returns status="error" with
// error_code="no_sections_data" instead of silently falling back to raw text.
//
// RED STATE: current code falls back to rawText and returns status="ok".
// This test will pass after T012 removes the silent fallback.
func TestHandlePreviewATSExtraction_NoSectionsSidecar_HardFails(t *testing.T) {
	cfg := stubApplyConfigForSession() // stubResumeRepo.LoadSections returns ErrSectionsMissing

	sessionID := buildScoredSession(t, &cfg)

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("preview_ats_extraction not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error (T012 must remove silent rawText fallback) — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "no_sections_data" {
		t.Errorf("expected error.code=no_sections_data, got %v — full: %s", errObj, text)
	}
	// Ensure no constructed_text is leaked through the data field.
	if data, ok := env["data"].(map[string]any); ok && data != nil {
		if data["constructed_text"] != nil && data["constructed_text"] != "" {
			t.Errorf("constructed_text must not be present on error, got: %v", data["constructed_text"])
		}
	}
}

// TestHandlePreviewATSExtraction_RenderFails_ReturnsRenderFailedCode asserts that
// when the PDF renderer fails the handler returns status="error" with
// error_code="render_failed".
//
// RED STATE: this test will not compile until T012 adds PDFRenderer to
// pipeline.ApplyConfig. The compile error is the expected red state.
func TestHandlePreviewATSExtraction_RenderFails_ReturnsRenderFailedCode(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader() // LoadSections succeeds
	// T012 must add: cfg.PDFRenderer = &stubPDFRenderer{failRender: true}
	cfg.PDFRenderer = &stubPDFRenderer{failRender: true}
	cfg.Extractor = &stubExtractor{failExtract: false}
	cfg.SurvivalDiffer = &stubSurvivalDiffer{}

	sessionID := buildScoredSession(t, &cfg)

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("preview_ats_extraction not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error (T012 must replace render fallback with hard-fail) — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "render_failed" {
		t.Errorf("expected error.code=render_failed, got %v — full: %s", errObj, text)
	}
}

// TestHandlePreviewATSExtraction_ExtractFails_ReturnsExtractFailedCode asserts that
// when the PDF extractor fails the handler returns status="error" with
// error_code="extract_failed".
//
// RED STATE: this test will not compile until T012 adds both PDFRenderer and
// Extractor to pipeline.ApplyConfig. The compile error is the expected red state.
func TestHandlePreviewATSExtraction_ExtractFails_ReturnsExtractFailedCode(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader() // LoadSections succeeds
	// T012 must add: cfg.PDFRenderer + cfg.Extractor to pipeline.ApplyConfig
	cfg.PDFRenderer = &stubPDFRenderer{failRender: false} // render succeeds, returns fake PDF bytes
	cfg.Extractor = &stubExtractor{failExtract: true}     // extract fails
	cfg.SurvivalDiffer = &stubSurvivalDiffer{}

	sessionID := buildScoredSession(t, &cfg)

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("preview_ats_extraction not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error (T012 must replace extract fallback with hard-fail) — full: %s", env["status"], text)
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "extract_failed" {
		t.Errorf("expected error.code=extract_failed, got %v — full: %s", errObj, text)
	}
}

// ── T014: keyword_survival presence tests (RED STATE until T015/T016) ─────────

// TestHandlePreviewATSExtraction_KeywordSurvivalPresent asserts that the
// preview_ats_extraction response includes a "keyword_survival" field with the
// expected structure: dropped, matched, and total_jd_keywords keys.
//
// RED STATE: previewData struct does not yet have a KeywordSurvival field.
// This test will pass after T015 implements survival.Service and T016 wires it.
func TestHandlePreviewATSExtraction_KeywordSurvivalPresent(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader()
	cfg.PDFRenderer = &stubPDFRenderer{failRender: false}
	cfg.Extractor = &stubExtractor{failExtract: false}
	// stubScorerWithKeywords echoes JD required/preferred into KeywordResult so the
	// survival diff sees a non-empty keyword list. Real survival service: stub extractor
	// returns "extracted text" which does not contain "go", so it lands in dropped.
	cfg.Scorer = &stubScorerWithKeywords{}
	cfg.SurvivalDiffer = survival.New()

	sessionID := buildScoredSession(t, &cfg)

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("preview_ats_extraction not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Fatalf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data field in response")
	}
	ks, ok := data["keyword_survival"]
	if !ok {
		t.Fatalf("keyword_survival missing from response data; got keys: %v", data)
	}
	ksMap, ok := ks.(map[string]any)
	if !ok {
		t.Fatalf("keyword_survival is not an object: %T", ks)
	}
	// total_jd_keywords must be 1 (JD has required:["go"], preferred:[]).
	total, _ := ksMap["total_jd_keywords"].(float64)
	if total != 1 {
		t.Errorf("keyword_survival.total_jd_keywords = %v, want 1", ksMap["total_jd_keywords"])
	}
	dropped, _ := ksMap["dropped"].([]any)
	matched, _ := ksMap["matched"].([]any)
	// dropped + matched must equal total.
	if len(dropped)+len(matched) != int(total) {
		t.Errorf("dropped(%d) + matched(%d) = %d, want total(%d)", len(dropped), len(matched), len(dropped)+len(matched), int(total))
	}
	// stub extractor returns "extracted text" which does not contain "go" as a whole word,
	// so "go" must appear in dropped.
	foundInDropped := false
	for _, d := range dropped {
		if d == "go" {
			foundInDropped = true
		}
	}
	if !foundInDropped {
		t.Errorf("keyword 'go' should be in dropped (stub extractor returns 'extracted text'), got dropped=%v matched=%v", dropped, matched)
	}
}

// TestHandlePreviewATSExtraction_NilPDFRenderer_ReturnsConfigurationError verifies
// that a nil PDFRenderer returns a clean configuration_error instead of panicking.
func TestHandlePreviewATSExtraction_NilPDFRenderer_ReturnsConfigurationError(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader()
	cfg.PDFRenderer = nil
	cfg.Extractor = &stubExtractor{failExtract: false}
	cfg.SurvivalDiffer = &stubSurvivalDiffer{}

	sessionID := buildScoredSession(t, &cfg)

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error for nil PDFRenderer", env["status"])
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "configuration_error" {
		t.Errorf("expected error.code=configuration_error, got %v", errObj)
	}
}

// TestHandlePreviewATSExtraction_NilExtractor_ReturnsConfigurationError verifies
// that a nil Extractor returns a clean configuration_error instead of panicking.
func TestHandlePreviewATSExtraction_NilExtractor_ReturnsConfigurationError(t *testing.T) {
	cfg := stubApplyConfigWithSkillsLoader()
	cfg.PDFRenderer = &stubPDFRenderer{failRender: false}
	cfg.Extractor = nil
	cfg.SurvivalDiffer = &stubSurvivalDiffer{}

	sessionID := buildScoredSession(t, &cfg)

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error for nil Extractor", env["status"])
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "configuration_error" {
		t.Errorf("expected error.code=configuration_error, got %v", errObj)
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
