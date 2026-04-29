package onboarding_test

import (
	"context"
	"encoding/json"
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
	wantStored := map[string]bool{"ref:skills": true, "accomplishments:onboard": true}
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
	if result.Summary.AccomplishmentsCount != 1 {
		t.Errorf("AccomplishmentsCount = %d, want 1", result.Summary.AccomplishmentsCount)
	}
	// 1 resume + skills + accomplishments:onboard = 3 chunks
	if result.Summary.TotalChunks != 3 {
		t.Errorf("TotalChunks = %d, want 3", result.Summary.TotalChunks)
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

	check(filepath.Join(dataDir, "inputs", "backend.txt"), "resume content")
	check(filepath.Join(dataDir, "skills.md"), "skills content")

	// Accomplishments are stored in accomplishments.json; onboard_text holds the raw text.
	accPath := filepath.Join(dataDir, "accomplishments.json")
	accData, err := os.ReadFile(accPath) // #nosec G304 -- test reads temp dir
	if err != nil {
		t.Fatalf("read accomplishments.json: %v", err)
	}
	var acc model.AccomplishmentsJSON
	if err := json.Unmarshal(accData, &acc); err != nil {
		t.Fatalf("unmarshal accomplishments.json: %v", err)
	}
	if acc.OnboardText != "accomplishments content" {
		t.Errorf("onboard_text = %q, want %q", acc.OnboardText, "accomplishments content")
	}

	// No legacy split-file chunks should be created.
	matches, _ := filepath.Glob(filepath.Join(dataDir, "accomplishments-*.md"))
	if len(matches) > 0 {
		t.Errorf("legacy accomplishments-N.md files found: %v", matches)
	}
}

func TestOnboardingService_CreatedStoriesPreservedOnReOnboard(t *testing.T) {
	dataDir := t.TempDir()
	svc := onboarding.New(dataDir, slog.Default())

	// Pre-seed accomplishments.json with existing created_stories.
	seed := model.AccomplishmentsJSON{
		SchemaVersion: "1",
		OnboardText:   "old onboard text",
		CreatedStories: []model.CreatedStory{
			{
				ID:       "story-001",
				Skill:    "Go",
				Type:     model.StoryTypeAchievement,
				JobTitle: "Senior Engineer",
				Text:     "Reduced latency by 40%.",
			},
		},
	}
	seedData, err := json.Marshal(seed)
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "accomplishments.json"), seedData, 0o600); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	// Re-onboard with new text.
	_, err = svc.Run(context.Background(), model.OnboardInput{
		AccomplishmentsText: "new onboard text",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dataDir, "accomplishments.json")) // #nosec G304 -- test reads temp dir
	if err != nil {
		t.Fatalf("read accomplishments.json: %v", err)
	}
	var result model.AccomplishmentsJSON
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// onboard_text must be updated.
	if result.OnboardText != "new onboard text" {
		t.Errorf("onboard_text = %q, want %q", result.OnboardText, "new onboard text")
	}

	// created_stories must be preserved unchanged.
	if len(result.CreatedStories) != 1 {
		t.Fatalf("created_stories len = %d, want 1", len(result.CreatedStories))
	}
	if result.CreatedStories[0].ID != "story-001" {
		t.Errorf("created_stories[0].ID = %q, want %q", result.CreatedStories[0].ID, "story-001")
	}
	if result.CreatedStories[0].Text != "Reduced latency by 40%." {
		t.Errorf("created_stories[0].Text = %q, want preserved value", result.CreatedStories[0].Text)
	}
}

func TestOnboardingService_CorruptAccomplishmentsJSONReturnsError(t *testing.T) {
	dataDir := t.TempDir()
	svc := onboarding.New(dataDir, slog.Default())

	// Write corrupt JSON.
	if err := os.WriteFile(filepath.Join(dataDir, "accomplishments.json"), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	_, err := svc.Run(context.Background(), model.OnboardInput{
		AccomplishmentsText: "some text",
	})
	if err == nil {
		t.Fatal("expected error for corrupt accomplishments.json, got nil")
	}
}

func TestOnboardingService_AtomicWritePreservesOriginalOnFailure(t *testing.T) {
	dataDir := t.TempDir()
	svc := onboarding.New(dataDir, slog.Default())

	// Write known-good accomplishments.json.
	original := model.AccomplishmentsJSON{
		SchemaVersion: "1",
		OnboardText:   "original text",
	}
	originalData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal original: %v", err)
	}
	accPath := filepath.Join(dataDir, "accomplishments.json")
	if err := os.WriteFile(accPath, originalData, 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}

	// Make dataDir read-only so any write (including the tmp file) fails.
	if err := os.Chmod(dataDir, 0o500); err != nil {
		t.Fatalf("chmod dataDir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dataDir, 0o700)
	})

	_, runErr := svc.Run(context.Background(), model.OnboardInput{
		AccomplishmentsText: "new text",
	})
	if runErr == nil {
		t.Fatal("expected error when dataDir is read-only, got nil")
	}

	// Restore permissions to read the file.
	if err := os.Chmod(dataDir, 0o700); err != nil {
		t.Fatalf("restore chmod: %v", err)
	}

	data, err := os.ReadFile(accPath) // #nosec G304 -- test reads temp dir
	if err != nil {
		t.Fatalf("read accomplishments.json after failure: %v", err)
	}
	var result model.AccomplishmentsJSON
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal after failure: %v", err)
	}
	if result.OnboardText != "original text" {
		t.Errorf("onboard_text = %q after write failure, want original %q", result.OnboardText, "original text")
	}
}
