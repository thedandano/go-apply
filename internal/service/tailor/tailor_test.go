package tailor_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/tailor"
)

// stubLLM is a test double that returns a fixed response.
type stubLLM struct{ response string }

var _ port.LLMClient = (*stubLLM)(nil)

func (s *stubLLM) ChatComplete(_ context.Context, _ []port.ChatMessage, _ port.ChatOptions) (string, error) {
	return s.response, nil
}

// errLLM always returns an error.
type errLLM struct{}

var _ port.LLMClient = (*errLLM)(nil)

func (e *errLLM) ChatComplete(_ context.Context, _ []port.ChatMessage, _ port.ChatOptions) (string, error) {
	return "", fmt.Errorf("llm error")
}

func makeDefaults() *config.AppDefaults {
	d, _ := config.LoadDefaults()
	return d
}

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

func TestTailorResume_Tier1Only(t *testing.T) {
	svc := tailor.New(&stubLLM{}, makeDefaults())
	input := port.TailorInput{
		Resume:     model.ResumeFile{Label: "backend"},
		ResumeText: "Skills\nLanguages: Python\n",
		JD:         model.JDData{},
		ScoreBefore: model.ScoreResult{
			Keywords: model.KeywordResult{ReqUnmatched: []string{"golang"}},
		},
		Options: port.TailorOptions{MaxTier2BulletRewrites: 0},
	}
	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TierApplied != model.TierKeyword {
		t.Errorf("TierApplied = %v, want TierKeyword", result.TierApplied)
	}
	if len(result.AddedKeywords) == 0 {
		t.Error("expected keywords to be added")
	}
}

func TestTailorResume_Tier2LLMError_Degrades(t *testing.T) {
	svc := tailor.New(&errLLM{}, makeDefaults())
	input := port.TailorInput{
		Resume:              model.ResumeFile{Label: "backend"},
		ResumeText:          "Skills\nLanguages: Python\n\nExperience\n- Built systems\n",
		JD:                  model.JDData{},
		ScoreBefore:         model.ScoreResult{Keywords: model.KeywordResult{ReqUnmatched: []string{"golang"}}},
		AccomplishmentsText: "Led a team to deliver Go microservices",
		Options:             port.TailorOptions{MaxTier2BulletRewrites: 2},
	}
	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("LLM error should degrade, not fail: %v", err)
	}
	// Tier-1 should still have run.
	if result.TierApplied < model.TierKeyword {
		t.Errorf("expected at least TierKeyword after degrade, got %v", result.TierApplied)
	}
}

func TestTailorResume_Tier2RewritesBullets(t *testing.T) {
	stub := &stubLLM{response: "- Designed distributed systems using golang and kubernetes\n"}
	svc := tailor.New(stub, makeDefaults())
	resumeText := "Skills\nLanguages: Python\n\nExperience\n- Built microservices\n"
	input := port.TailorInput{
		Resume:     model.ResumeFile{Label: "backend"},
		ResumeText: resumeText,
		JD:         model.JDData{},
		ScoreBefore: model.ScoreResult{
			Keywords: model.KeywordResult{
				ReqUnmatched: []string{"golang", "kubernetes"},
			},
		},
		AccomplishmentsText: "Led design of distributed Go microservices serving 10M users",
		Options:             port.TailorOptions{MaxTier2BulletRewrites: 1},
	}
	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TierApplied != model.TierBullet {
		t.Errorf("TierApplied = %v, want TierBullet", result.TierApplied)
	}
	if len(result.RewrittenBullets) == 0 {
		t.Fatal("expected at least one rewritten bullet")
	}
	// Verify the marker was preserved (original used "- ")
	if !strings.Contains(result.TailoredText, "Designed distributed systems") {
		t.Errorf("tailored text does not contain rewritten content\nText: %s", result.TailoredText)
	}
	// Verify original bullet is gone
	if strings.Contains(result.TailoredText, "Built microservices") {
		t.Errorf("original bullet still present in tailored text")
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
