package pipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/presenter/headless"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

// stubResumeRepo satisfies port.ResumeRepository.
type stubResumeRepo struct{}

func (s *stubResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return []model.ResumeFile{{Label: "test", Path: "/fake/resume.txt"}}, nil
}

// stubDocumentLoader satisfies port.DocumentLoader.
type stubDocumentLoader struct{}

func (s *stubDocumentLoader) Supports(_ string) bool { return true }
func (s *stubDocumentLoader) Load(_ string) (string, error) {
	return "golang kubernetes docker python experience 5 years senior engineer", nil
}

// stubAppRepo — ApplicationRepository, always cache miss on Get.
type stubAppRepo struct{}

func (s *stubAppRepo) Get(_ string) (*model.ApplicationRecord, bool, error) { return nil, false, nil }
func (s *stubAppRepo) Put(_ *model.ApplicationRecord) error                 { return nil }
func (s *stubAppRepo) Update(_ *model.ApplicationRecord) error              { return nil }
func (s *stubAppRepo) List() ([]*model.ApplicationRecord, error)            { return nil, nil }

var _ port.ApplicationRepository = (*stubAppRepo)(nil)

// stubAugmentService — pass-through.
type stubAugmentService struct{}

var _ port.Augmenter = (*stubAugmentService)(nil)

func (s *stubAugmentService) AugmentResumeText(_ context.Context, input model.AugmentInput) (string, *model.ReferenceData, error) {
	return input.ResumeText, input.RefData, nil
}

// stubCoverLetter — fixed cover letter.
type stubCoverLetter struct{}

var _ port.CoverLetterGenerator = (*stubCoverLetter)(nil)

func (s *stubCoverLetter) Generate(_ context.Context, _ *model.CoverLetterInput) (model.CoverLetterResult, error) {
	return model.CoverLetterResult{Text: "Cover letter.", Channel: model.ChannelCold, SentenceCount: 1}, nil
}

// stubJDFetcher.
type stubJDFetcher struct{}

var _ port.JDFetcher = (*stubJDFetcher)(nil)

func (s *stubJDFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return "fake JD text", nil
}

// stubTailorForApply satisfies port.Tailor with a fixed result for use in ApplyPipeline tests.
type stubTailorForApply struct{}

var _ port.Tailor = (*stubTailorForApply)(nil)

func (s *stubTailorForApply) TailorResume(_ context.Context, _ *model.TailorInput) (model.TailorResult, error) {
	return model.TailorResult{
		TierApplied:   model.TierKeyword,
		AddedKeywords: []string{"golang"},
		TailoredText:  "tailored resume text",
	}, nil
}

// stubTailorError satisfies port.Tailor and always returns an error to exercise degradation.
type stubTailorError struct{}

var _ port.Tailor = (*stubTailorError)(nil)

func (s *stubTailorError) TailorResume(_ context.Context, _ *model.TailorInput) (model.TailorResult, error) {
	return model.TailorResult{}, fmt.Errorf("llm unavailable")
}

func TestApplyPipeline_HeadlessE2E(t *testing.T) {
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": `{"title":"SWE","company":"Acme","required":["python","golang","kubernetes"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`,
				}},
			},
		})
	}))
	defer llmSrv.Close()

	var stdout, stderr bytes.Buffer
	pres := headless.NewWith(&stdout, &stderr)

	cfg := &config.Config{
		Orchestrator:      config.LLMProviderConfig{BaseURL: llmSrv.URL, Model: "test", APIKey: "test"},
		YearsOfExperience: 5,
		DefaultSeniority:  "exact",
	}

	defaults, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	llmClient := llm.New(llmSrv.URL, "test", "test", defaults, nil)

	pl := pipeline.NewApplyPipeline(&pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       llmClient,
		Scorer:    scorer.New(defaults),
		CLGen:     &stubCoverLetter{},
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubAppRepo{},
		Augment:   &stubAugmentService{},
		Presenter: pres,
		Defaults:  defaults,
		Tailor:    nil,
	})

	err = pl.Run(context.Background(), pipeline.ApplyRequest{
		URLOrText: `We are hiring a senior Go engineer. Required: python, golang, kubernetes. Preferred: docker.`,
		IsText:    true,
		Channel:   model.ChannelCold,
		Config:    cfg,
	})

	if err != nil {
		t.Fatalf("pipeline error: %v", err)
	}

	var result model.PipelineResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, stdout.String())
	}
	if result.Status != "success" {
		t.Errorf("status = %q, want success", result.Status)
	}
	if result.JDText == "" {
		t.Error("expected JDText to be populated, got empty string")
	}
	if result.BestScore == 0 {
		t.Error("best_score is 0 — scoring did not run")
	}
}

func TestApplyPipeline_TailorStep(t *testing.T) {
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": `{"title":"SWE","company":"Acme","required":["python","golang","kubernetes"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`,
				}},
			},
		})
	}))
	defer llmSrv.Close()

	var stdout, stderr bytes.Buffer
	pres := headless.NewWith(&stdout, &stderr)

	cfg := &config.Config{
		Orchestrator:      config.LLMProviderConfig{BaseURL: llmSrv.URL, Model: "test", APIKey: "test"},
		YearsOfExperience: 5,
		DefaultSeniority:  "senior",
	}

	defaults, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	llmClient := llm.New(llmSrv.URL, "test", "test", defaults, nil)

	pl := pipeline.NewApplyPipeline(&pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       llmClient,
		Scorer:    scorer.New(defaults),
		CLGen:     &stubCoverLetter{},
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubAppRepo{},
		Augment:   &stubAugmentService{},
		Presenter: pres,
		Defaults:  defaults,
		Tailor:    &stubTailorForApply{},
	})

	err = pl.Run(context.Background(), pipeline.ApplyRequest{
		URLOrText:           `We are hiring a senior Go engineer. Required: python, golang, kubernetes. Preferred: docker.`,
		IsText:              true,
		Channel:             model.ChannelCold,
		Config:              cfg,
		AccomplishmentsText: "accomplishments text",
	})
	if err != nil {
		t.Fatalf("pipeline error: %v", err)
	}

	var result model.PipelineResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, stdout.String())
	}
	if result.Cascade == nil {
		t.Fatal("result.Cascade is nil — tailor step did not run")
	}
	found := false
	for _, kw := range result.Cascade.AddedKeywords {
		if kw == "golang" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("result.Cascade.AddedKeywords = %v, want to contain %q", result.Cascade.AddedKeywords, "golang")
	}
}

func TestApplyPipeline_TailorStep_TailorError(t *testing.T) {
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": `{"title":"SWE","company":"Acme","required":["python","golang","kubernetes"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`,
				}},
			},
		})
	}))
	defer llmSrv.Close()

	var stdout, stderr bytes.Buffer
	pres := headless.NewWith(&stdout, &stderr)

	cfg := &config.Config{
		Orchestrator:      config.LLMProviderConfig{BaseURL: llmSrv.URL, Model: "test", APIKey: "test"},
		YearsOfExperience: 5,
		DefaultSeniority:  "senior",
	}

	defaults, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	llmClient := llm.New(llmSrv.URL, "test", "test", defaults, nil)

	pl := pipeline.NewApplyPipeline(&pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       llmClient,
		Scorer:    scorer.New(defaults),
		CLGen:     &stubCoverLetter{},
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubAppRepo{},
		Augment:   &stubAugmentService{},
		Presenter: pres,
		Defaults:  defaults,
		Tailor:    &stubTailorError{},
	})

	err = pl.Run(context.Background(), pipeline.ApplyRequest{
		URLOrText:           `We are hiring a senior Go engineer. Required: python, golang, kubernetes. Preferred: docker.`,
		IsText:              true,
		Channel:             model.ChannelCold,
		Config:              cfg,
		AccomplishmentsText: "accomplishments text",
	})
	if err != nil {
		t.Fatalf("pipeline error: %v — expected degradation (nil error), not a fatal failure", err)
	}

	var result model.PipelineResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, stdout.String())
	}
	if result.Cascade != nil {
		t.Errorf("result.Cascade = %+v, want nil — tailor error should not produce output", result.Cascade)
	}
	if len(result.Warnings) == 0 {
		t.Error("result.Warnings is empty — expected a warning to be recorded on tailor error")
	}
}
