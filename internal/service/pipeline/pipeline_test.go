package pipeline_test

import (
	"context"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
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

// stubJDFetcher satisfies port.JDFetcher.
type stubJDFetcher struct{}

var _ port.JDFetcher = (*stubJDFetcher)(nil)

func (s *stubJDFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return "fake JD text from url", nil
}

// capturingPresenter captures the ShowResult call.
type capturingPresenter struct {
	result *model.PipelineResult
	err    error
}

func (p *capturingPresenter) ShowResult(r *model.PipelineResult) error {
	p.result = r
	return nil
}
func (p *capturingPresenter) ShowError(err error)                          { p.err = err }
func (p *capturingPresenter) OnEvent(_ any)                                {}
func (p *capturingPresenter) ShowTailorResult(_ *model.TailorResult) error { return nil }

var _ port.Presenter = (*capturingPresenter)(nil)

func minimalApplyConfig(pres *capturingPresenter) *pipeline.ApplyConfig {
	defaults, _ := config.LoadDefaults()
	return &pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		Scorer:    scorer.New(defaults),
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubAppRepo{},
		Presenter: pres,
		Defaults:  defaults,
	}
}

func TestScoreResumes_ReturnsScoresAndBest(t *testing.T) {
	pres := &capturingPresenter{}
	cfg := minimalApplyConfig(pres)
	pl := pipeline.NewApplyPipeline(cfg)

	jd := model.JDData{
		Title:     "Go Engineer",
		Company:   "Acme",
		Required:  []string{"golang", "kubernetes"},
		Preferred: []string{"docker"},
	}
	result, err := pl.ScoreResumes(context.Background(), &jd, &config.Config{YearsOfExperience: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Scores) == 0 {
		t.Error("ScoreResumes returned no scores")
	}
	if result.BestLabel == "" {
		t.Error("ScoreResumes returned empty BestLabel")
	}
}

func TestAcquireJD_TextInput_PassesThrough(t *testing.T) {
	pres := &capturingPresenter{}
	cfg := minimalApplyConfig(pres)
	pl := pipeline.NewApplyPipeline(cfg)

	text, err := pl.AcquireJD(context.Background(), "raw jd text", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "raw jd text" {
		t.Errorf("AcquireJD = %q, want %q", text, "raw jd text")
	}
}

func TestAcquireJD_URLInput_FetchesText(t *testing.T) {
	pres := &capturingPresenter{}
	cfg := minimalApplyConfig(pres)
	pl := pipeline.NewApplyPipeline(cfg)

	text, err := pl.AcquireJD(context.Background(), "https://example.com/job", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text == "" {
		t.Error("AcquireJD returned empty text for URL input")
	}
}

func TestScoreResume_SingleResume_ReturnsScore(t *testing.T) {
	pres := &capturingPresenter{}
	cfg := minimalApplyConfig(pres)
	pl := pipeline.NewApplyPipeline(cfg)

	jd := model.JDData{
		Title:     "Go Engineer",
		Company:   "Acme",
		Required:  []string{"golang"},
		Preferred: []string{"docker"},
	}
	result, err := pl.ScoreResume(
		context.Background(),
		"golang kubernetes experience senior",
		"test",
		&jd,
		&config.Config{YearsOfExperience: 5, DefaultSeniority: "senior"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Breakdown.Total() == 0 {
		t.Error("ScoreResume returned zero total score")
	}
}
