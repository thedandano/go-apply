package tailor

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
)

func defaultsForTest(t *testing.T) *config.AppDefaults {
	t.Helper()
	d, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	return d
}

func newTestService(llm *stubLLMClient, defaults *config.AppDefaults) *Service {
	return New(llm, defaults, slog.Default())
}

func TestTailorResume_Tier1Only(t *testing.T) {
	defaults := defaultsForTest(t)
	defaults.Tailor.MaxTier2BulletRewrites = 0 // tier-2 disabled via options

	resume := `## Skills
Python

## Experience

- Built distributed systems
`
	input := &model.TailorInput{
		Resume:     model.ResumeFile{Label: "main"},
		ResumeText: resume,
		JD: model.JDData{
			Required:  []string{"Golang"},
			Preferred: []string{"Kubernetes"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 0},
	}

	svc := newTestService(&stubLLMClient{response: "no-op"}, defaults)
	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("TailorResume: %v", err)
	}

	if result.TierApplied != model.TierKeyword {
		t.Errorf("tier = %v, want TierKeyword", result.TierApplied)
	}
	if len(result.RewrittenBullets) != 0 {
		t.Errorf("expected no rewritten bullets for tier-1 only, got %d", len(result.RewrittenBullets))
	}
	if result.TailoredText == "" {
		t.Error("TailoredText must not be empty")
	}
	if !strings.Contains(result.TailoredText, "Golang") {
		t.Error("TailoredText should contain the injected keyword Golang")
	}
	if result.ResumeLabel != "main" {
		t.Errorf("ResumeLabel = %q, want 'main'", result.ResumeLabel)
	}
}

func TestTailorResume_Tier2LLMError_Degrades(t *testing.T) {
	defaults := defaultsForTest(t)
	defaults.Tailor.MaxTier2BulletRewrites = 5

	resume := `## Skills
Python

## Experience

- Deployed Kubernetes clusters at scale
`
	input := &model.TailorInput{
		Resume:              model.ResumeFile{Label: "main"},
		ResumeText:          resume,
		AccomplishmentsText: "Managed large Kubernetes deployments",
		JD: model.JDData{
			Required: []string{"Kubernetes"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 5},
	}

	// LLM always errors on every bullet call.
	// rewriteBullets swallows per-bullet errors and skips them, producing zero
	// changes. TailorResume then keeps the tier-1 result (TierKeyword) unchanged.
	// This is the defined internal-degradation path — no error is surfaced.
	svc := newTestService(&stubLLMClient{err: errors.New("llm unavailable")}, defaults)
	result, err := svc.TailorResume(context.Background(), input)

	// Must degrade gracefully — no error returned.
	if err != nil {
		t.Fatalf("expected no error on tier-2 LLM failure, got: %v", err)
	}
	// TierApplied must remain TierKeyword: no bullet changes were produced.
	if result.TierApplied != model.TierKeyword {
		t.Errorf("tier = %v, want TierKeyword when all tier-2 bullet rewrites fail", result.TierApplied)
	}
	// TailoredText must be the tier-1 output (non-empty) containing the injected keyword.
	if result.TailoredText == "" {
		t.Error("TailoredText must not be empty even when tier-2 degrades")
	}
	// Tier-1 should have injected "Kubernetes" into the Skills section.
	if !strings.Contains(result.TailoredText, "Kubernetes") {
		t.Errorf("TailoredText should contain tier-1 injected keyword 'Kubernetes'; got:\n%s", result.TailoredText)
	}
	// No rewritten bullets: tier-2 produced zero changes.
	if len(result.RewrittenBullets) != 0 {
		t.Errorf("expected no rewritten bullets when LLM fails, got %d", len(result.RewrittenBullets))
	}
}

func TestTailorResume_Tier2RewritesBullets(t *testing.T) {
	defaults := defaultsForTest(t)
	defaults.Tailor.MaxTier2BulletRewrites = 5

	resume := `## Skills
Python

## Experience

- Led Kubernetes migration for 50-node cluster
`
	input := &model.TailorInput{
		Resume:              model.ResumeFile{Label: "main"},
		ResumeText:          resume,
		AccomplishmentsText: "Completed Kubernetes migration 2 weeks ahead of schedule",
		JD: model.JDData{
			Required: []string{"Kubernetes"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 5},
	}

	stub := &stubLLMClient{response: "- Led Kubernetes migration of 50-node cluster 2 weeks ahead of schedule"}
	svc := newTestService(stub, defaults)
	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("TailorResume: %v", err)
	}

	if result.TierApplied != model.TierBullet {
		t.Errorf("tier = %v, want TierBullet", result.TierApplied)
	}
	if len(result.RewrittenBullets) == 0 {
		t.Error("expected at least one rewritten bullet")
	}
	if result.TailoredText == "" {
		t.Error("TailoredText must be set")
	}
	// Verify TailoredText contains the rewritten content.
	if !strings.Contains(result.TailoredText, "ahead of schedule") {
		t.Errorf("TailoredText does not contain rewritten bullet content; got:\n%s", result.TailoredText)
	}
}
