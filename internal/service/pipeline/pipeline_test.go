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

func (s *stubResumeRepo) LoadSections(_ string) (model.SectionMap, error) {
	return model.SectionMap{}, model.ErrSectionsMissing
}

func (s *stubResumeRepo) SaveSections(_ string, _ model.SectionMap) error { return nil } //nolint:gocritic // hugeParam: interface constraint

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

// stubJDFetcher.
type stubJDFetcher struct{}

var _ port.JDFetcher = (*stubJDFetcher)(nil)

func (s *stubJDFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return "fake JD text", nil
}

// noopPresenter satisfies port.Presenter.
type noopPresenter struct{}

func (p *noopPresenter) OnEvent(_ any) {}

var _ port.Presenter = (*noopPresenter)(nil)

func minimalApplyConfig() *pipeline.ApplyConfig {
	defaults, _ := config.LoadDefaults()
	return &pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		Scorer:    scorer.New(defaults),
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubAppRepo{},
		Presenter: &noopPresenter{},
		Defaults:  defaults,
	}
}

func TestAcquireJD_TextInput_PassesThrough(t *testing.T) {
	cfg := minimalApplyConfig()
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
	cfg := minimalApplyConfig()
	pl := pipeline.NewApplyPipeline(cfg)

	text, err := pl.AcquireJD(context.Background(), "https://example.com/job", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text == "" {
		t.Error("AcquireJD returned empty text for URL input")
	}
}
