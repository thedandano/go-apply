package onboarding_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/onboarding"
)

func newService(t *testing.T) *onboarding.Service {
	t.Helper()
	return onboarding.New(t.TempDir(), slog.Default())
}

func TestOnboardingService_ResumeStoredOnDisk(t *testing.T) {
	dataDir := t.TempDir()
	svc := onboarding.New(dataDir, slog.Default())

	result, err := svc.Run(context.Background(), model.OnboardInput{
		Resumes: []model.ResumeEntry{{Label: "backend", Text: "Go engineer resume"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	if len(result.Stored) != 1 || result.Stored[0] != "resume:backend" {
		t.Errorf("Stored = %v, want [resume:backend]", result.Stored)
	}

	// Resume must be in inputs/ subdirectory.
	resumePath := filepath.Join(dataDir, "inputs", "backend.txt")
	data, err := os.ReadFile(resumePath) // #nosec G304 -- test reads temp dir
	if err != nil {
		t.Fatalf("read resume file: %v", err)
	}
	if string(data) != "Go engineer resume" {
		t.Errorf("resume content = %q, want %q", data, "Go engineer resume")
	}
}

func TestOnboardingService_SkillsAndAccomplishmentsStored(t *testing.T) {
	svc := newService(t)

	result, err := svc.Run(context.Background(), model.OnboardInput{
		SkillsText:          "Go, Python, Docker",
		AccomplishmentsText: "Led team of 5 engineers",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Warnings) > 0 {
		t.Errorf("unexpected warnings: %v", result.Warnings)
	}
	wantStored := map[string]bool{"ref:skills": true, "accomplishments:0": true}
	for _, s := range result.Stored {
		delete(wantStored, s)
	}
	if len(wantStored) > 0 {
		t.Errorf("missing from Stored: %v", wantStored)
	}
}

func TestOnboardingService_RejectsTraversalLabel(t *testing.T) {
	svc := newService(t)

	result, err := svc.Run(context.Background(), model.OnboardInput{
		Resumes: []model.ResumeEntry{{Label: "../etc/passwd", Text: "malicious"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for traversal label")
	}
	if len(result.Stored) > 0 {
		t.Error("traversal label must not result in stored document")
	}
}

func TestOnboardingService_RejectsSlashLabel(t *testing.T) {
	svc := newService(t)

	result, err := svc.Run(context.Background(), model.OnboardInput{
		Resumes: []model.ResumeEntry{{Label: "foo/bar", Text: "text"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for slash label")
	}
}

func TestOnboardingService_RejectsEmptyLabel(t *testing.T) {
	svc := newService(t)

	result, err := svc.Run(context.Background(), model.OnboardInput{
		Resumes: []model.ResumeEntry{{Label: "", Text: "text"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning for empty label")
	}
	if len(result.Stored) > 0 {
		t.Error("empty label must not result in stored document")
	}
}

func TestOnboardingService_SummaryPopulated(t *testing.T) {
	skills := "Go\nPython\nDocker"
	accomplishments := "## Scaled backend\nLed team of 5 engineers\n\n## Reduced latency\nCut p99 from 800ms to 120ms"

	svc := newService(t)

	result, err := svc.Run(context.Background(), model.OnboardInput{
		Resumes:             []model.ResumeEntry{{Label: "backend", Text: "Go engineer resume"}},
		SkillsText:          skills,
		AccomplishmentsText: accomplishments,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Summary.ResumesAdded != 1 {
		t.Errorf("ResumesAdded = %d, want 1", result.Summary.ResumesAdded)
	}
	if result.Summary.SkillsCount != 3 {
		t.Errorf("SkillsCount = %d, want 3", result.Summary.SkillsCount)
	}
	if result.Summary.AccomplishmentsCount != 2 {
		t.Errorf("AccomplishmentsCount = %d, want 2", result.Summary.AccomplishmentsCount)
	}
	// 1 resume + skills + 2 accomplishment sections = 4 chunks
	if result.Summary.TotalChunks != 4 {
		t.Errorf("TotalChunks = %d, want 4", result.Summary.TotalChunks)
	}
}

func TestOnboardingService_SummaryResumesAddedOnlyCountsSuccessful(t *testing.T) {
	svc := newService(t)

	// One valid resume and one invalid (path traversal) — only the valid one should count.
	result, err := svc.Run(context.Background(), model.OnboardInput{
		Resumes: []model.ResumeEntry{
			{Label: "backend", Text: "Go engineer resume"},
			{Label: "../etc/passwd", Text: "malicious"},
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Summary.ResumesAdded != 1 {
		t.Errorf("ResumesAdded = %d, want 1 (only successful stores count)", result.Summary.ResumesAdded)
	}
}

func TestOnboardingService_WritesFilesToDisk(t *testing.T) {
	dataDir := t.TempDir()
	svc := onboarding.New(dataDir, slog.Default())

	_, err := svc.Run(context.Background(), model.OnboardInput{
		Resumes:             []model.ResumeEntry{{Label: "backend", Text: "resume content"}},
		SkillsText:          "skills content",
		AccomplishmentsText: "accomplishments content",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	check := func(path, want string) {
		t.Helper()
		data, err := os.ReadFile(path) // #nosec G304 -- test reads temp dir
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(data) != want {
			t.Errorf("%s: got %q, want %q", path, data, want)
		}
	}

	// Resumes go in inputs/; skills and accomplishments go in dataDir root.
	check(filepath.Join(dataDir, "inputs", "backend.txt"), "resume content")
	check(filepath.Join(dataDir, "skills.md"), "skills content")
	check(filepath.Join(dataDir, "accomplishments-0.md"), "accomplishments content")
}
