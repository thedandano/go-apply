package cli

// Handler unit tests for handleApplyToJob and handleGetScore.
// These are internal tests (package cli) because both handlers are unexported.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// ─── builder helpers ─────────────────────────────────────────────────────────

func makeHandlerDefaults(t *testing.T) *config.AppDefaults {
	t.Helper()
	d, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	return d
}

func makeHandlerCfg() *config.Config {
	return &config.Config{
		DefaultSeniority:  "senior",
		YearsOfExperience: 5,
		UserName:          "Test User",
		Occupation:        "Engineer",
	}
}

// ─── stub port implementations ───────────────────────────────────────────────

type handlerStubFetcher struct{}

var _ port.JDFetcher = (*handlerStubFetcher)(nil)

func (s *handlerStubFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return "Software Engineer job posting requiring Go", nil
}

// ─────────────────────────────────────────────────────────────────────────────

type handlerStubLLM struct{}

var _ port.LLMClient = (*handlerStubLLM)(nil)

func (s *handlerStubLLM) ChatComplete(_ context.Context, _ []port.ChatMessage, _ port.ChatOptions) (string, error) {
	return `{"title":"Go Engineer","company":"Acme","required":["Go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`, nil
}

// ─────────────────────────────────────────────────────────────────────────────

type handlerStubScorer struct{}

var _ port.Scorer = (*handlerStubScorer)(nil)

func (s *handlerStubScorer) Score(_ port.ScorerInput) (model.ScoreResult, error) {
	return model.ScoreResult{
		Breakdown: model.ScoreBreakdown{
			KeywordMatch:   30,
			ExperienceFit:  20,
			ImpactEvidence: 5,
			ATSFormat:      5,
			Readability:    5,
		},
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────

type handlerStubCoverLetter struct{}

var _ port.CoverLetterGenerator = (*handlerStubCoverLetter)(nil)

func (s *handlerStubCoverLetter) Generate(_ context.Context, _ *port.CoverLetterInput) (model.CoverLetterResult, error) {
	return model.CoverLetterResult{Text: "Dear Hiring Manager,", WordCount: 3, SentenceCount: 1}, nil
}

// ─────────────────────────────────────────────────────────────────────────────

type handlerStubResumeRepo struct {
	resumes []model.ResumeFile
}

var _ port.ResumeRepository = (*handlerStubResumeRepo)(nil)

func (s *handlerStubResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return s.resumes, nil
}

// ─────────────────────────────────────────────────────────────────────────────

type handlerStubJDCache struct{}

var _ port.JDCacheRepository = (*handlerStubJDCache)(nil)

func (s *handlerStubJDCache) Get(_ string) (string, model.JDData, bool) {
	return "", model.JDData{}, false
}
func (s *handlerStubJDCache) Put(_ string, _ string, _ model.JDData) error { return nil }
func (s *handlerStubJDCache) Update(_ string, _ model.JDData) error        { return nil }

// ─────────────────────────────────────────────────────────────────────────────

type handlerStubLoader struct{}

var _ port.DocumentLoader = (*handlerStubLoader)(nil)

func (s *handlerStubLoader) Load(_ string) (string, error) {
	return "Experienced Go engineer with Kubernetes skills", nil
}

func (s *handlerStubLoader) Supports(_ string) bool { return true }

// ─── pipeline builder factories ──────────────────────────────────────────────

// happyBuilder returns a pipelineBuilder wired with successful stubs.
func happyBuilder(t *testing.T) pipelineBuilder {
	t.Helper()
	defaults := makeHandlerDefaults(t)
	cfg := makeHandlerCfg()
	return func(_ string, pres port.Presenter) *pipeline.ApplyPipeline {
		return pipeline.New(pipeline.Config{
			Fetcher: &handlerStubFetcher{},
			LLM:     &handlerStubLLM{},
			Scorer:  &handlerStubScorer{},
			CLGen:   &handlerStubCoverLetter{},
			Resumes: &handlerStubResumeRepo{resumes: []model.ResumeFile{
				{Label: "test.pdf", Path: "/tmp/test.pdf", FileType: ".pdf"},
			}},
			JDCache:   &handlerStubJDCache{},
			Augmenter: nil,
			DocLoader: &handlerStubLoader{},
			Presenter: pres,
			Defaults:  defaults,
			Cfg:       cfg,
		})
	}
}

// errorBuilder returns a pipelineBuilder wired with an empty resume repo to force
// a fatal pipeline error (ShowError + non-nil return from Run).
func errorBuilder(t *testing.T) pipelineBuilder {
	t.Helper()
	defaults := makeHandlerDefaults(t)
	cfg := makeHandlerCfg()
	return func(_ string, pres port.Presenter) *pipeline.ApplyPipeline {
		return pipeline.New(pipeline.Config{
			Fetcher:   &handlerStubFetcher{},
			LLM:       &handlerStubLLM{},
			Scorer:    &handlerStubScorer{},
			CLGen:     &handlerStubCoverLetter{},
			Resumes:   &handlerStubResumeRepo{resumes: []model.ResumeFile{}},
			JDCache:   &handlerStubJDCache{},
			Augmenter: nil,
			DocLoader: &handlerStubLoader{},
			Presenter: pres,
			Defaults:  defaults,
			Cfg:       cfg,
		})
	}
}

// ─── CallToolRequest helper ───────────────────────────────────────────────────

func makeReq(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

// ─── errorJSON helper ─────────────────────────────────────────────────────────

// errorJSON parses the "error" field from the tool result content.
func errorJSON(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("CallToolResult has no content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(text.Text), &m); err != nil {
		t.Fatalf("unmarshal tool result text %q: %v", text.Text, err)
	}
	return m["error"]
}

// resultText returns the raw JSON text from the tool result.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("CallToolResult has no content")
	}
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return text.Text
}

// ─── tailor stub ─────────────────────────────────────────────────────────────

type handlerStubTailor struct{}

var _ port.Tailor = (*handlerStubTailor)(nil)

func (s *handlerStubTailor) TailorResume(_ context.Context, input port.TailorInput) (model.TailorResult, error) {
	return model.TailorResult{
		ResumeLabel:   input.Resume.Label,
		TierApplied:   model.TierKeyword,
		AddedKeywords: []string{"Go", "Kubernetes"},
		TailoredText:  input.ResumeText + " [tailored]",
	}, nil
}

// ─── tailor pipeline builder factories ───────────────────────────────────────

// tailorHappyBuilder returns a tailorPipelineBuilder wired with successful stubs.
func tailorHappyBuilder(t *testing.T) tailorPipelineBuilder {
	t.Helper()
	defaults := makeHandlerDefaults(t)
	return func(_ string, pres port.Presenter) *pipeline.TailorPipeline {
		return pipeline.NewTailorPipeline(pipeline.TailorConfig{
			Fetcher: &handlerStubFetcher{},
			LLM:     &handlerStubLLM{},
			Scorer:  &handlerStubScorer{},
			Tailor:  &handlerStubTailor{},
			Resumes: &handlerStubResumeRepo{resumes: []model.ResumeFile{
				{Label: "backend", Path: "/tmp/backend.pdf", FileType: ".pdf"},
			}},
			JDCache:   &handlerStubJDCache{},
			Augmenter: nil,
			DocLoader: &handlerStubLoader{},
			Presenter: pres,
			Defaults:  defaults,
		})
	}
}

// tailorErrorBuilder returns a tailorPipelineBuilder that fails (empty resume repo).
func tailorErrorBuilder(t *testing.T) tailorPipelineBuilder {
	t.Helper()
	defaults := makeHandlerDefaults(t)
	return func(_ string, pres port.Presenter) *pipeline.TailorPipeline {
		return pipeline.NewTailorPipeline(pipeline.TailorConfig{
			Fetcher:   &handlerStubFetcher{},
			LLM:       &handlerStubLLM{},
			Scorer:    &handlerStubScorer{},
			Tailor:    &handlerStubTailor{},
			Resumes:   &handlerStubResumeRepo{resumes: []model.ResumeFile{}},
			JDCache:   &handlerStubJDCache{},
			Augmenter: nil,
			DocLoader: &handlerStubLoader{},
			Presenter: pres,
			Defaults:  defaults,
		})
	}
}

// ─── handleApplyToJob tests ───────────────────────────────────────────────────

// TestHandleApplyToJob_BothEmpty verifies that omitting both jd_url and jd_text
// returns a validation error JSON with no Go error.
func TestHandleApplyToJob_BothEmpty(t *testing.T) {
	req := makeReq(map[string]any{})
	result, err := handleApplyToJob(context.Background(), req, happyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	got := errorJSON(t, result)
	if !strings.Contains(got, "exactly one") {
		t.Errorf("error %q does not mention 'exactly one'", got)
	}
}

// TestHandleApplyToJob_BothProvided verifies that supplying both jd_url and jd_text
// returns a mutual-exclusion error JSON.
func TestHandleApplyToJob_BothProvided(t *testing.T) {
	req := makeReq(map[string]any{
		"jd_url":  "https://example.com/job",
		"jd_text": "some raw text",
	})
	result, err := handleApplyToJob(context.Background(), req, happyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	got := errorJSON(t, result)
	if !strings.Contains(got, "mutually exclusive") {
		t.Errorf("error %q does not mention 'mutually exclusive'", got)
	}
}

// TestHandleApplyToJob_InvalidChannel verifies that an unrecognised channel value
// returns an invalid-channel error response. The raw text is checked with a
// string-contains match because the channel value is embedded directly in the
// format string and the text may not be strict JSON.
func TestHandleApplyToJob_InvalidChannel(t *testing.T) {
	req := makeReq(map[string]any{
		"jd_text": "some job description",
		"channel": "INVALID",
	})
	result, err := handleApplyToJob(context.Background(), req, happyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	raw := resultText(t, result)
	if !strings.Contains(raw, "invalid channel") {
		t.Errorf("result text %q does not mention 'invalid channel'", raw)
	}
}

// TestHandleApplyToJob_PresenterError verifies that a fatal pipeline error
// (ShowError called by the pipeline) surfaces as a JSON error response.
// When the pipeline calls ShowError AND returns an error, pres.Err() is non-nil,
// so the handler returns an error JSON via the presenter-error branch.
func TestHandleApplyToJob_PresenterError(t *testing.T) {
	req := makeReq(map[string]any{
		"jd_text": "some job description",
	})
	result, err := handleApplyToJob(context.Background(), req, errorBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error (business errors are JSON), got: %v", err)
	}
	got := errorJSON(t, result)
	if got == "" {
		t.Error("expected non-empty error in JSON, got empty string")
	}
}

// TestHandleApplyToJob_HappyPath verifies that a successful run returns a JSON
// object with a non-zero best_score and no error field.
func TestHandleApplyToJob_HappyPath(t *testing.T) {
	req := makeReq(map[string]any{
		"jd_text": "Senior Go Engineer requiring Kubernetes experience",
		"channel": "COLD",
	})
	result, err := handleApplyToJob(context.Background(), req, happyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	raw := resultText(t, result)

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v\nbody: %s", err, raw)
	}
	if _, hasErr := m["error"]; hasErr {
		t.Errorf("unexpected error in result JSON: %s", raw)
	}
	bestScore, ok := m["best_score"].(float64)
	if !ok || bestScore == 0 {
		t.Errorf("expected non-zero best_score, got: %v", m["best_score"])
	}
}

// ─── handleGetScore tests ─────────────────────────────────────────────────────

// TestHandleGetScore_BothEmpty verifies that omitting both jd_url and jd_text
// returns a validation error JSON.
func TestHandleGetScore_BothEmpty(t *testing.T) {
	req := makeReq(map[string]any{})
	result, err := handleGetScore(context.Background(), req, happyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	got := errorJSON(t, result)
	if !strings.Contains(got, "exactly one") {
		t.Errorf("error %q does not mention 'exactly one'", got)
	}
}

// TestHandleGetScore_BothProvided verifies that supplying both jd_url and jd_text
// returns a mutual-exclusion error JSON.
func TestHandleGetScore_BothProvided(t *testing.T) {
	req := makeReq(map[string]any{
		"jd_url":  "https://example.com/job",
		"jd_text": "some raw text",
	})
	result, err := handleGetScore(context.Background(), req, happyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	got := errorJSON(t, result)
	if !strings.Contains(got, "mutually exclusive") {
		t.Errorf("error %q does not mention 'mutually exclusive'", got)
	}
}

// TestHandleGetScore_PresenterError verifies that a fatal pipeline error
// surfaces as a JSON error response.
func TestHandleGetScore_PresenterError(t *testing.T) {
	req := makeReq(map[string]any{
		"jd_text": "some job description",
	})
	result, err := handleGetScore(context.Background(), req, errorBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error (business errors are JSON), got: %v", err)
	}
	got := errorJSON(t, result)
	if got == "" {
		t.Error("expected non-empty error in JSON, got empty string")
	}
}

// TestHandleGetScore_HappyPath verifies that a successful run returns a JSON
// object with a non-zero best_score and no error field.
func TestHandleGetScore_HappyPath(t *testing.T) {
	req := makeReq(map[string]any{
		"jd_text": "Senior Go Engineer requiring Kubernetes experience",
	})
	result, err := handleGetScore(context.Background(), req, happyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	raw := resultText(t, result)

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v\nbody: %s", err, raw)
	}
	if _, hasErr := m["error"]; hasErr {
		t.Errorf("unexpected error in result JSON: %s", raw)
	}
	bestScore, ok := m["best_score"].(float64)
	if !ok || bestScore == 0 {
		t.Errorf("expected non-zero best_score, got: %v", m["best_score"])
	}
}

// ─── handleTailorResume tests ─────────────────────────────────────────────────

// TestHandleTailorResume_MissingResumeLabel verifies that omitting resume_label
// returns a validation error JSON mentioning "resume_label".
func TestHandleTailorResume_MissingResumeLabel(t *testing.T) {
	req := makeReq(map[string]any{
		"jd_text": "some job description",
		// resume_label omitted
	})
	cfg := makeHandlerCfg()
	result, err := handleTailorResume(context.Background(), req, cfg, tailorHappyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	got := errorJSON(t, result)
	if !strings.Contains(got, "resume_label") {
		t.Errorf("error %q does not mention 'resume_label'", got)
	}
}

// TestHandleTailorResume_BothJDEmpty verifies that omitting both jd_url and jd_text
// returns a validation error JSON mentioning "exactly one".
func TestHandleTailorResume_BothJDEmpty(t *testing.T) {
	req := makeReq(map[string]any{
		"resume_label": "backend",
		// jd_url and jd_text omitted
	})
	cfg := makeHandlerCfg()
	result, err := handleTailorResume(context.Background(), req, cfg, tailorHappyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	got := errorJSON(t, result)
	if !strings.Contains(got, "exactly one") {
		t.Errorf("error %q does not mention 'exactly one'", got)
	}
}

// TestHandleTailorResume_BothJDProvided verifies that supplying both jd_url and jd_text
// returns a mutual-exclusion error JSON.
func TestHandleTailorResume_BothJDProvided(t *testing.T) {
	req := makeReq(map[string]any{
		"resume_label": "backend",
		"jd_url":       "https://example.com/job",
		"jd_text":      "some raw text",
	})
	cfg := makeHandlerCfg()
	result, err := handleTailorResume(context.Background(), req, cfg, tailorHappyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	got := errorJSON(t, result)
	if !strings.Contains(got, "mutually exclusive") {
		t.Errorf("error %q does not mention 'mutually exclusive'", got)
	}
}

// TestHandleTailorResume_PipelineError verifies that a fatal pipeline error
// (e.g. resume not found in empty repo) surfaces as a JSON error response.
func TestHandleTailorResume_PipelineError(t *testing.T) {
	req := makeReq(map[string]any{
		"resume_label": "backend",
		"jd_text":      "some job description",
	})
	cfg := makeHandlerCfg()
	result, err := handleTailorResume(context.Background(), req, cfg, tailorErrorBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error (business errors are JSON), got: %v", err)
	}
	got := errorJSON(t, result)
	if got == "" {
		t.Error("expected non-empty error in JSON, got empty string")
	}
}

// TestHandleTailorResume_HappyPath verifies that a successful run returns a valid
// JSON object with a resume_label field and no error field.
func TestHandleTailorResume_HappyPath(t *testing.T) {
	req := makeReq(map[string]any{
		"resume_label": "backend",
		"jd_text":      "Senior Go Engineer requiring Kubernetes experience",
	})
	cfg := makeHandlerCfg()
	result, err := handleTailorResume(context.Background(), req, cfg, tailorHappyBuilder(t))
	if err != nil {
		t.Fatalf("expected nil Go error, got: %v", err)
	}
	raw := resultText(t, result)

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("result is not valid JSON: %v\nbody: %s", err, raw)
	}
	if _, hasErr := m["error"]; hasErr {
		t.Errorf("unexpected error in result JSON: %s", raw)
	}
	if m["resume_label"] == nil {
		t.Errorf("expected resume_label in result, got: %s", raw)
	}
}
