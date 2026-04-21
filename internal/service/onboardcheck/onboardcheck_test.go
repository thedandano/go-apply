package onboardcheck_test

import (
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/onboardcheck"
)

// stubEmptyResumeRepo returns no resumes, simulating an unonboarded user.
type stubEmptyResumeRepo struct{}

func (s *stubEmptyResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return nil, nil
}

// stubResumeRepo returns a populated list, simulating an onboarded user.
type stubResumeRepo struct{}

func (s *stubResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return []model.ResumeFile{{Label: "resume", Path: "/tmp/resume.pdf", FileType: "pdf"}}, nil
}

func TestCheckOnboarded_EmbedderNotConfigured_ReturnsError(t *testing.T) {
	cfg := &config.Config{} // zero-value: embedder.base_url and embedder.model empty
	err := onboardcheck.CheckOnboarded(cfg, &stubResumeRepo{})
	if err == nil {
		t.Fatal("expected error when embedder is not configured, got nil")
	}
	if !strings.Contains(err.Error(), "embedder not configured") {
		t.Errorf("error = %q, want to contain 'embedder not configured'", err.Error())
	}
}

func TestCheckOnboarded_EmbedderMissingModel_ReturnsError(t *testing.T) {
	cfg := &config.Config{}
	cfg.Embedder.BaseURL = "http://localhost:11434/v1"
	// model left empty
	err := onboardcheck.CheckOnboarded(cfg, &stubResumeRepo{})
	if err == nil {
		t.Fatal("expected error when embedder model is not configured, got nil")
	}
	if !strings.Contains(err.Error(), "embedder not configured") {
		t.Errorf("error = %q, want to contain 'embedder not configured'", err.Error())
	}
}

func TestCheckOnboarded_NoResumes_ReturnsError(t *testing.T) {
	cfg := &config.Config{}
	cfg.Embedder.BaseURL = "http://localhost:11434/v1"
	cfg.Embedder.Model = "nomic-embed-text"

	err := onboardcheck.CheckOnboarded(cfg, &stubEmptyResumeRepo{})
	if err == nil {
		t.Fatal("expected error when no resumes found, got nil")
	}
	if !strings.Contains(err.Error(), "no resumes found") {
		t.Errorf("error = %q, want to contain 'no resumes found'", err.Error())
	}
}

func TestCheckOnboarded_Onboarded_ReturnsNil(t *testing.T) {
	cfg := &config.Config{}
	cfg.Embedder.BaseURL = "http://localhost:11434/v1"
	cfg.Embedder.Model = "nomic-embed-text"

	err := onboardcheck.CheckOnboarded(cfg, &stubResumeRepo{})
	if err != nil {
		t.Errorf("expected nil when onboarded, got: %v", err)
	}
}
