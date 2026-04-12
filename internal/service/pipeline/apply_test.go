package pipeline_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// ─── stub implementations ─────────────────────────────────────────────────────

type stubFetcher struct{}

var _ port.JDFetcher = (*stubFetcher)(nil)

func (s *stubFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return "Software Engineer job posting requiring Go and Kubernetes", nil
}

// ─────────────────────────────────────────────────────────────────────────────

type stubLLM struct {
	response string
	err      error
}

var _ port.LLMClient = (*stubLLM)(nil)

func (s *stubLLM) ChatComplete(_ context.Context, _ []port.ChatMessage, _ port.ChatOptions) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.response != "" {
		return s.response, nil
	}
	return `{"title":"Software Engineer","company":"Acme","required":["Go","Kubernetes"],"preferred":["Docker"],"location":"Remote","seniority":"senior","required_years":5}`, nil
}

// ─────────────────────────────────────────────────────────────────────────────

type stubScorer struct{}

var _ port.Scorer = (*stubScorer)(nil)

func (s *stubScorer) Score(_ port.ScorerInput) (model.ScoreResult, error) {
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

type stubCoverLetter struct {
	err error
}

var _ port.CoverLetterGenerator = (*stubCoverLetter)(nil)

func (s *stubCoverLetter) Generate(_ context.Context, _ *port.CoverLetterInput) (model.CoverLetterResult, error) {
	if s.err != nil {
		return model.CoverLetterResult{}, s.err
	}
	return model.CoverLetterResult{Text: "Dear Hiring Manager,", WordCount: 3, SentenceCount: 1}, nil
}

// ─────────────────────────────────────────────────────────────────────────────

type stubResumeRepo struct {
	resumes []model.ResumeFile
}

var _ port.ResumeRepository = (*stubResumeRepo)(nil)

func (s *stubResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return s.resumes, nil
}

// ─────────────────────────────────────────────────────────────────────────────

type stubJDCache struct{}

var _ port.JDCacheRepository = (*stubJDCache)(nil)

func (s *stubJDCache) Get(_ string) (string, model.JDData, bool)    { return "", model.JDData{}, false }
func (s *stubJDCache) Put(_ string, _ string, _ model.JDData) error { return nil }
func (s *stubJDCache) Update(_ string, _ model.JDData) error        { return nil }

// ─────────────────────────────────────────────────────────────────────────────

type stubLoader struct{}

var _ port.DocumentLoader = (*stubLoader)(nil)

func (s *stubLoader) Load(_ string) (string, error) {
	return "Experienced Go engineer with Kubernetes and Docker skills", nil
}

func (s *stubLoader) Supports(_ string) bool { return true }

// ─────────────────────────────────────────────────────────────────────────────

// stubPresenter captures ShowResult/ShowError calls for assertions.
type stubPresenter struct {
	ResultCalled *model.PipelineResult
	ErrorCalled  error
	events       []any
}

var _ port.Presenter = (*stubPresenter)(nil)

func (s *stubPresenter) OnEvent(event any) { s.events = append(s.events, event) }

func (s *stubPresenter) ShowResult(result *model.PipelineResult) error {
	s.ResultCalled = result
	return nil
}

func (s *stubPresenter) ShowTailorResult(_ *model.TailorResult) error { return nil }

func (s *stubPresenter) ShowError(err error) {
	s.ErrorCalled = err
}

// ─────────────────────────────────────────────────────────────────────────────

func makeDefaults() *config.AppDefaults {
	d, _ := config.LoadDefaults()
	return d
}

func makeCfg() *config.Config {
	return &config.Config{
		DefaultSeniority:  "senior",
		YearsOfExperience: 7,
		UserName:          "Alice",
		Occupation:        "Software Engineer",
	}
}

func makeTestResumes() []model.ResumeFile {
	return []model.ResumeFile{
		{Label: "alice.pdf", Path: "/tmp/alice.pdf", FileType: ".pdf"},
	}
}

// ─────────────────────────────────────────────────────────────────────────────

// TestApplyPipeline_Run_TextInput verifies that a text-mode run scores resumes
// and calls presenter.ShowResult with a non-zero BestScore.
func TestApplyPipeline_Run_TextInput(t *testing.T) {
	presenter := &stubPresenter{}
	p := pipeline.New(pipeline.Config{
		Fetcher:   &stubFetcher{},
		LLM:       &stubLLM{},
		Scorer:    &stubScorer{},
		CLGen:     &stubCoverLetter{},
		Resumes:   &stubResumeRepo{resumes: makeTestResumes()},
		JDCache:   &stubJDCache{},
		Augmenter: nil,
		DocLoader: &stubLoader{},
		Presenter: presenter,
		Defaults:  makeDefaults(),
		Cfg:       makeCfg(),
	})

	err := p.Run(context.Background(), pipeline.RunInput{
		Text:    "Senior Go Engineer requiring Kubernetes experience",
		Channel: model.ChannelCold,
	})

	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if presenter.ResultCalled == nil {
		t.Fatal("ShowResult was not called")
	}
	if presenter.ResultCalled.BestScore == 0 {
		t.Errorf("expected non-zero BestScore, got 0")
	}
	if presenter.ResultCalled.BestResume == "" {
		t.Errorf("expected non-empty BestResume")
	}
}

// TestApplyPipeline_Run_NoResumes verifies that an empty resume list triggers
// ShowError and returns an error.
func TestApplyPipeline_Run_NoResumes(t *testing.T) {
	presenter := &stubPresenter{}
	p := pipeline.New(pipeline.Config{
		Fetcher:   &stubFetcher{},
		LLM:       &stubLLM{},
		Scorer:    &stubScorer{},
		CLGen:     &stubCoverLetter{},
		Resumes:   &stubResumeRepo{resumes: []model.ResumeFile{}},
		JDCache:   &stubJDCache{},
		Augmenter: nil,
		DocLoader: &stubLoader{},
		Presenter: presenter,
		Defaults:  makeDefaults(),
		Cfg:       makeCfg(),
	})

	err := p.Run(context.Background(), pipeline.RunInput{
		Text:    "Some job posting",
		Channel: model.ChannelCold,
	})

	if err == nil {
		t.Fatal("expected error when no resumes, got nil")
	}
	if presenter.ErrorCalled == nil {
		t.Error("ShowError was not called")
	}
	if presenter.ResultCalled != nil {
		t.Error("ShowResult should not be called when no resumes")
	}
}

// TestApplyPipeline_Run_KeywordExtractionDegrades verifies that LLM failure for
// keyword extraction does not kill the pipeline — a warning is added and scoring proceeds.
func TestApplyPipeline_Run_KeywordExtractionDegrades(t *testing.T) {
	presenter := &stubPresenter{}
	p := pipeline.New(pipeline.Config{
		Fetcher:   &stubFetcher{},
		LLM:       &stubLLM{err: errors.New("LLM unavailable")},
		Scorer:    &stubScorer{},
		CLGen:     &stubCoverLetter{},
		Resumes:   &stubResumeRepo{resumes: makeTestResumes()},
		JDCache:   &stubJDCache{},
		Augmenter: nil,
		DocLoader: &stubLoader{},
		Presenter: presenter,
		Defaults:  makeDefaults(),
		Cfg:       makeCfg(),
	})

	err := p.Run(context.Background(), pipeline.RunInput{
		Text:    "Senior Go Engineer requiring Kubernetes experience",
		Channel: model.ChannelCold,
	})

	if err != nil {
		t.Fatalf("Run should not return error on keyword degradation, got: %v", err)
	}
	if presenter.ResultCalled == nil {
		t.Fatal("ShowResult was not called")
	}
	if len(presenter.ResultCalled.Warnings) == 0 {
		t.Error("expected at least one warning for keyword extraction failure")
	}
	// Score should still be set from stub scorer
	if presenter.ResultCalled.BestScore == 0 {
		t.Errorf("expected non-zero BestScore even after keyword degradation, got 0")
	}
}

// TestApplyPipeline_Run_CoverLetterDegrades verifies that cover letter failure
// adds a warning but still returns a result.
func TestApplyPipeline_Run_CoverLetterDegrades(t *testing.T) {
	presenter := &stubPresenter{}
	p := pipeline.New(pipeline.Config{
		Fetcher:   &stubFetcher{},
		LLM:       &stubLLM{},
		Scorer:    &stubScorer{},
		CLGen:     &stubCoverLetter{err: errors.New("cover letter LLM error")},
		Resumes:   &stubResumeRepo{resumes: makeTestResumes()},
		JDCache:   &stubJDCache{},
		Augmenter: nil,
		DocLoader: &stubLoader{},
		Presenter: presenter,
		Defaults:  makeDefaults(),
		Cfg:       makeCfg(),
	})

	err := p.Run(context.Background(), pipeline.RunInput{
		Text:    "Senior Go Engineer requiring Kubernetes experience",
		Channel: model.ChannelCold,
	})

	if err != nil {
		t.Fatalf("Run should not return error on cover letter failure, got: %v", err)
	}
	if presenter.ResultCalled == nil {
		t.Fatal("ShowResult was not called")
	}
	if len(presenter.ResultCalled.Warnings) == 0 {
		t.Error("expected at least one warning for cover letter failure")
	}
	if presenter.ResultCalled.Status != "degraded" {
		t.Errorf("status: got %q, want %q", presenter.ResultCalled.Status, "degraded")
	}
	// Cover letter text should be empty (degraded)
	if presenter.ResultCalled.CoverLetter.Text != "" {
		t.Errorf("expected empty cover letter text on failure, got %q", presenter.ResultCalled.CoverLetter.Text)
	}
}

// stubFailAugmenter always returns an error to test degraded mode.
type stubFailAugmenter struct{}

var _ port.Augmenter = (*stubFailAugmenter)(nil)

func (s *stubFailAugmenter) AugmentResumeText(_ context.Context, _ port.AugmentInput) (string, *port.ReferenceData, error) {
	return "", nil, errors.New("embedding unavailable")
}

// TestApplyPipeline_Run_AugmentDegrades verifies that augmentation failure adds a
// warning, sets status to "degraded", and continues the pipeline with the original text.
func TestApplyPipeline_Run_AugmentDegrades(t *testing.T) {
	pres := &stubPresenter{}

	p := pipeline.New(pipeline.Config{
		Fetcher:   &stubFetcher{},
		LLM:       &stubLLM{},
		Scorer:    &stubScorer{},
		CLGen:     &stubCoverLetter{},
		Resumes:   &stubResumeRepo{resumes: makeTestResumes()},
		JDCache:   &stubJDCache{},
		Augmenter: &stubFailAugmenter{},
		DocLoader: &stubLoader{},
		Presenter: pres,
		Defaults:  makeDefaults(),
		Cfg:       makeCfg(),
	})

	err := p.Run(context.Background(), pipeline.RunInput{
		Text:    "Senior Go Engineer requiring Kubernetes experience",
		Channel: model.ChannelCold,
	})

	if err != nil {
		t.Fatalf("Run should not return error on augment failure, got: %v", err)
	}
	if pres.ResultCalled == nil {
		t.Fatal("ShowResult was not called")
	}
	if len(pres.ResultCalled.Warnings) == 0 {
		t.Error("expected at least one warning for augmentation failure")
	}
	if pres.ResultCalled.Status != "degraded" {
		t.Errorf("status: got %q, want %q", pres.ResultCalled.Status, "degraded")
	}
}
