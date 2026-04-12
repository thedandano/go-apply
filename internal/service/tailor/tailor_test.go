package tailor_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/service/tailor"
)

const resumeWithSkills = `Dan Sedano
Software Engineer

Summary
Experienced engineer with Go and Python skills.

Skills
Go
Python
Docker

Experience
- Built microservices using Go
- Deployed containers with Docker

Education
BS Computer Science`

func TestTier1AddKeywords_AddsToSkillsSection(t *testing.T) {
	missing := []string{"Kubernetes", "Terraform"}
	result, added := tailor.AddKeywordsToSkillsSection(resumeWithSkills, missing)

	if len(added) != 2 {
		t.Fatalf("expected 2 added keywords, got %d: %v", len(added), added)
	}
	if added[0] != "Kubernetes" || added[1] != "Terraform" {
		t.Errorf("unexpected added keywords: %v", added)
	}

	for _, kw := range missing {
		if !contains(result, kw) {
			t.Errorf("expected %q to appear in result text", kw)
		}
	}
}

func TestTier1AddKeywords_DoesNotDuplicate(t *testing.T) {
	// "Go" and "Docker" already exist in the resume.
	missing := []string{"Go", "Docker", "Kubernetes"}
	_, added := tailor.AddKeywordsToSkillsSection(resumeWithSkills, missing)

	for _, kw := range added {
		if kw == "Go" || kw == "Docker" {
			t.Errorf("keyword %q already present in resume but was returned as added", kw)
		}
	}

	// Only Kubernetes should have been added.
	if len(added) != 1 || added[0] != "Kubernetes" {
		t.Errorf("expected only [Kubernetes] to be added, got %v", added)
	}
}

func TestTier1AddKeywords_NoSkillsSection(t *testing.T) {
	noSkills := `Dan Sedano
Software Engineer

Experience
- Built microservices
`
	original := noSkills
	result, added := tailor.AddKeywordsToSkillsSection(noSkills, []string{"Kubernetes", "Terraform"})

	if result != original {
		t.Error("expected text to be unchanged when no skills section is present")
	}
	if len(added) != 0 {
		t.Errorf("expected no added keywords, got %v", added)
	}
}

// contains is a helper to check for substring presence.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
