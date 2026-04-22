package onboardcheck_test

import (
	"strings"
	"testing"

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

func TestCheckOnboarded_NoResumes_ReturnsError(t *testing.T) {
	err := onboardcheck.CheckOnboarded(&stubEmptyResumeRepo{})
	if err == nil {
		t.Fatal("expected error when no resumes found, got nil")
	}
	if !strings.Contains(err.Error(), "no resumes found") {
		t.Errorf("error = %q, want to contain 'no resumes found'", err.Error())
	}
}

func TestCheckOnboarded_Onboarded_ReturnsNil(t *testing.T) {
	err := onboardcheck.CheckOnboarded(&stubResumeRepo{})
	if err != nil {
		t.Errorf("expected nil when onboarded, got: %v", err)
	}
}
