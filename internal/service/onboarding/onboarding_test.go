package onboarding_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/onboarding"
)

// Compile-time interface satisfaction checks.
var _ port.ProfileRepository = (*stubProfileRepo)(nil)
var _ port.EmbeddingClient = (*stubEmbedder)(nil)

// stubProfileRepo records UpsertDocument calls and optionally injects errors.
type stubProfileRepo struct {
	upserted map[string]string // sourceDoc → text
	failOn   string            // if non-empty, UpsertDocument returns error for this source
}

func (r *stubProfileRepo) UpsertDocument(_ context.Context, sourceDoc, text string, _ []float32) error {
	if r.failOn != "" && strings.HasPrefix(sourceDoc, r.failOn) {
		return errors.New("upsert: simulated failure")
	}
	if r.upserted == nil {
		r.upserted = make(map[string]string)
	}
	r.upserted[sourceDoc] = text
	return nil
}

func (r *stubProfileRepo) FindSimilar(_ context.Context, _ []float32, _ int) ([]model.ProfileEmbedding, error) {
	return nil, nil
}

func (r *stubProfileRepo) ListDocuments(_ context.Context) ([]model.ProfileDocument, error) {
	return nil, nil
}

// stubEmbedder returns a fixed vector or an error.
type stubEmbedder struct {
	fail bool
}

func (e *stubEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	if e.fail {
		return nil, errors.New("embed: simulated failure")
	}
	return []float32{0.1, 0.2, 0.3}, nil
}

func newService(t *testing.T, repo *stubProfileRepo, embedder *stubEmbedder) *onboarding.Service {
	t.Helper()
	dataDir := t.TempDir()
	return onboarding.New(repo, embedder, dataDir, slog.Default())
}

func TestOnboardingService_ResumeStoredAndIndexed(t *testing.T) {
	repo := &stubProfileRepo{}
	svc := newService(t, repo, &stubEmbedder{})

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
	if _, ok := repo.upserted["resume:backend"]; !ok {
		t.Error("resume:backend not upserted into profile repo")
	}
}

func TestOnboardingService_SkillsAndAccomplishmentsStored(t *testing.T) {
	repo := &stubProfileRepo{}
	svc := newService(t, repo, &stubEmbedder{})

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
	wantStored := map[string]bool{"ref:skills": true, "accomplishments": true}
	for _, s := range result.Stored {
		delete(wantStored, s)
	}
	if len(wantStored) > 0 {
		t.Errorf("missing from Stored: %v", wantStored)
	}
}

func TestOnboardingService_EmbedFailureDegrades(t *testing.T) {
	repo := &stubProfileRepo{}
	svc := newService(t, repo, &stubEmbedder{fail: true})

	result, err := svc.Run(context.Background(), model.OnboardInput{
		Resumes: []model.ResumeEntry{{Label: "backend", Text: "Go engineer resume"}},
	})
	if err != nil {
		t.Fatalf("Run must not return error on embed failure: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning on embed failure")
	}
	if len(result.Stored) > 0 {
		t.Errorf("nothing should be stored on embed failure, got: %v", result.Stored)
	}
}

func TestOnboardingService_UpsertFailureDegrades(t *testing.T) {
	repo := &stubProfileRepo{failOn: "resume:"}
	svc := newService(t, repo, &stubEmbedder{})

	result, err := svc.Run(context.Background(), model.OnboardInput{
		Resumes: []model.ResumeEntry{{Label: "backend", Text: "Go engineer resume"}},
	})
	if err != nil {
		t.Fatalf("Run must not return error on upsert failure: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning on upsert failure")
	}
}

func TestOnboardingService_RejectsTraversalLabel(t *testing.T) {
	repo := &stubProfileRepo{}
	svc := newService(t, repo, &stubEmbedder{})

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
	repo := &stubProfileRepo{}
	svc := newService(t, repo, &stubEmbedder{})

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
	repo := &stubProfileRepo{}
	svc := newService(t, repo, &stubEmbedder{})

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
	skills := "Go, Python, Docker"
	accomplishments := "Led team of 5 engineers"

	repo := &stubProfileRepo{}
	svc := newService(t, repo, &stubEmbedder{})

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
	if result.Summary.SkillsChars != len(skills) {
		t.Errorf("SkillsChars = %d, want %d", result.Summary.SkillsChars, len(skills))
	}
	if result.Summary.AccomplishmentsChars != len(accomplishments) {
		t.Errorf("AccomplishmentsChars = %d, want %d", result.Summary.AccomplishmentsChars, len(accomplishments))
	}
	// 1 resume + skills + accomplishments = 3 chunks
	if result.Summary.TotalChunks != 3 {
		t.Errorf("TotalChunks = %d, want 3", result.Summary.TotalChunks)
	}
}

func TestOnboardingService_SummaryResumesAddedOnlyCountsSuccessful(t *testing.T) {
	repo := &stubProfileRepo{}
	svc := newService(t, repo, &stubEmbedder{})

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
	repo := &stubProfileRepo{}
	dataDir := t.TempDir()
	svc := onboarding.New(repo, &stubEmbedder{}, dataDir, slog.Default())

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
	inputsDir := filepath.Join(dataDir, "inputs")
	check(filepath.Join(inputsDir, "backend.txt"), "resume content")
	check(filepath.Join(inputsDir, "skills.md"), "skills content")
	check(filepath.Join(inputsDir, "accomplishments.md"), "accomplishments content")
}
