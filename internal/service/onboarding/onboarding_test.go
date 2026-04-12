package onboarding_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/onboarding"
)

// Compile-time interface satisfaction checks.
var _ port.ProfileRepository = (*stubProfileRepo)(nil)
var _ port.EmbeddingClient = (*stubEmbedder)(nil)

type stubEmbedder struct {
	vec []float32
}

func (s *stubEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	if s.vec == nil {
		return []float32{0.1, 0.2, 0.3}, nil
	}
	return s.vec, nil
}

type upsertCall struct {
	sourceDoc string
	text      string
	vector    []float32
}

type stubProfileRepo struct {
	calls []upsertCall
}

func (s *stubProfileRepo) UpsertDocument(_ context.Context, sourceDoc string, text string, vector []float32) error {
	s.calls = append(s.calls, upsertCall{sourceDoc: sourceDoc, text: text, vector: vector})
	return nil
}

func (s *stubProfileRepo) FindSimilar(_ context.Context, _ []float32, _ int) ([]model.ProfileEmbedding, error) {
	return nil, nil
}

func TestOnboardingService_StoresResumeAndIndexes(t *testing.T) {
	embedder := &stubEmbedder{}
	repo := &stubProfileRepo{}
	dataDir := t.TempDir()

	svc := onboarding.New(repo, embedder, dataDir)

	input := onboarding.OnboardInput{
		Resumes: map[string]onboarding.OnboardFile{
			"backend": {
				Label:     "backend",
				PlainText: "Go engineer with 5 years of experience",
				OrigPath:  "/some/path/backend.pdf",
				Format:    ".pdf",
			},
		},
	}

	result, err := svc.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// assert inputs/<label>.txt written to temp dir
	destPath := filepath.Join(dataDir, "inputs", "backend.txt")
	data, readErr := os.ReadFile(destPath)
	if readErr != nil {
		t.Fatalf("expected inputs/backend.txt to exist, got: %v", readErr)
	}
	if string(data) != "Go engineer with 5 years of experience" {
		t.Errorf("unexpected file content: %q", string(data))
	}

	// assert UpsertDocument called at least once
	if len(repo.calls) == 0 {
		t.Fatal("expected UpsertDocument to be called at least once")
	}

	// assert result.ResumesStored contains label
	if len(result.ResumesStored) == 0 {
		t.Fatal("expected ResumesStored to contain 'backend'")
	}
	found := false
	for _, label := range result.ResumesStored {
		if label == "backend" {
			found = true
		}
	}
	if !found {
		t.Errorf("ResumesStored = %v, want to contain 'backend'", result.ResumesStored)
	}

	// assert result.EmbeddingsIndexed > 0
	if result.EmbeddingsIndexed == 0 {
		t.Error("expected EmbeddingsIndexed > 0")
	}
}

func TestOnboardingService_StoresSkillsAndAccomplishments(t *testing.T) {
	embedder := &stubEmbedder{}
	repo := &stubProfileRepo{}
	dataDir := t.TempDir()

	svc := onboarding.New(repo, embedder, dataDir)

	input := onboarding.OnboardInput{
		Resumes:             map[string]onboarding.OnboardFile{},
		SkillsText:          "Go, Python, Kubernetes",
		AccomplishmentsText: "Led team of 5 engineers",
	}

	result, err := svc.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	if !result.SkillsStored {
		t.Error("expected SkillsStored = true")
	}
	if !result.AccomplishmentsStored {
		t.Error("expected AccomplishmentsStored = true")
	}
	if result.EmbeddingsIndexed != 2 {
		t.Errorf("expected EmbeddingsIndexed = 2, got %d", result.EmbeddingsIndexed)
	}

	skillsPath := filepath.Join(dataDir, "skills.md")
	if _, err := os.Stat(skillsPath); err != nil {
		t.Errorf("expected skills.md to exist: %v", err)
	}
	accomplishmentsPath := filepath.Join(dataDir, "accomplishments.md")
	if _, err := os.Stat(accomplishmentsPath); err != nil {
		t.Errorf("expected accomplishments.md to exist: %v", err)
	}
}

func TestOnboardingService_WarnsOnEmbedError(t *testing.T) {
	repo := &stubProfileRepo{}
	dataDir := t.TempDir()

	// Use an embedder that fails
	failEmbedder := &failingEmbedder{}

	svc := onboarding.New(repo, failEmbedder, dataDir)

	input := onboarding.OnboardInput{
		Resumes: map[string]onboarding.OnboardFile{
			"backend": {
				Label:     "backend",
				PlainText: "some resume text",
			},
		},
	}

	result, err := svc.Run(context.Background(), input)
	if err != nil {
		t.Fatalf("Run() should not return fatal error, got: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warnings when embedding fails")
	}
	if result.EmbeddingsIndexed != 0 {
		t.Errorf("expected EmbeddingsIndexed = 0, got %d", result.EmbeddingsIndexed)
	}
	// Repo should not have been called
	if len(repo.calls) != 0 {
		t.Errorf("expected no upsert calls when embed fails, got %d", len(repo.calls))
	}
}

var _ port.EmbeddingClient = (*failingEmbedder)(nil)

type failingEmbedder struct{}

func (f *failingEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("embed failed")
}
