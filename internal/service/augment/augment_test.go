package augment_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/augment"
)

// stubEmbedder returns a fixed vector regardless of input.
type stubEmbedder struct {
	vec []float32
	err error
}

var _ port.EmbeddingClient = (*stubEmbedder)(nil)

func (s *stubEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return s.vec, s.err
}

// stubProfileRepo returns fixed similar documents.
type stubProfileRepo struct {
	results []model.ProfileEmbedding
	err     error
}

var _ port.ProfileRepository = (*stubProfileRepo)(nil)

func (s *stubProfileRepo) UpsertDocument(_ context.Context, _ string, _ string, _ []float32) error {
	return nil
}

func (s *stubProfileRepo) FindSimilar(_ context.Context, _ []float32, _ int) ([]model.ProfileEmbedding, error) {
	return s.results, s.err
}

func defaults() *config.AppDefaults {
	d, _ := config.LoadDefaults()
	return d
}

func TestAugmentResumeText_AppendsRelevantChunks(t *testing.T) {
	repo := &stubProfileRepo{
		results: []model.ProfileEmbedding{
			{ID: 1, SourceDoc: "ref:skills", Term: "Led a team of 5 engineers", Weight: 0.9},
			{ID: 2, SourceDoc: "ref:projects", Term: "Built distributed systems", Weight: 0.85},
		},
	}
	embedder := &stubEmbedder{vec: []float32{0.1, 0.2, 0.3}}

	svc := augment.New(repo, embedder, defaults())

	input := port.AugmentInput{
		ResumeText: "Original resume text",
		RefData:    nil,
		JDKeywords: []string{"leadership", "distributed systems"},
	}
	augmented, refData, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("AugmentResumeText: %v", err)
	}
	if !strings.Contains(augmented, "Original resume text") {
		t.Error("augmented text should contain original resume text")
	}
	if !strings.Contains(augmented, "Led a team of 5 engineers") {
		t.Error("augmented text should contain first profile chunk")
	}
	if !strings.Contains(augmented, "Built distributed systems") {
		t.Error("augmented text should contain second profile chunk")
	}
	if refData == nil {
		t.Error("expected non-nil refData")
	}
}

func TestAugmentResumeText_EmptyKeywords_DegradeGracefully(t *testing.T) {
	repo := &stubProfileRepo{results: []model.ProfileEmbedding{}}
	embedder := &stubEmbedder{vec: []float32{0.0, 0.0, 0.0}}

	svc := augment.New(repo, embedder, defaults())

	input := port.AugmentInput{
		ResumeText: "Original text",
		JDKeywords: []string{},
	}
	augmented, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error for empty keywords, got: %v", err)
	}
	if augmented != "Original text" {
		t.Errorf("expected original text unchanged, got: %q", augmented)
	}
}

func TestAugmentResumeText_EmbedderError_DegradeGracefully(t *testing.T) {
	repo := &stubProfileRepo{results: []model.ProfileEmbedding{}}
	embedder := &stubEmbedder{err: errors.New("embedding service unavailable")}

	svc := augment.New(repo, embedder, defaults())

	input := port.AugmentInput{
		ResumeText: "Original text",
		JDKeywords: []string{"go", "kubernetes"},
	}
	augmented, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error on embedder failure (degrade), got: %v", err)
	}
	if augmented != "Original text" {
		t.Errorf("expected original text on embedder error, got: %q", augmented)
	}
}

func TestAugmentResumeText_RepoError_DegradeGracefully(t *testing.T) {
	repo := &stubProfileRepo{err: errors.New("db connection lost")}
	embedder := &stubEmbedder{vec: []float32{0.1, 0.2}}

	svc := augment.New(repo, embedder, defaults())

	input := port.AugmentInput{
		ResumeText: "Original text",
		JDKeywords: []string{"go"},
	}
	augmented, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error on repo failure (degrade), got: %v", err)
	}
	if augmented != "Original text" {
		t.Errorf("expected original text on repo error, got: %q", augmented)
	}
}

func TestAugmentResumeText_BelowThreshold_NotAppended(t *testing.T) {
	repo := &stubProfileRepo{
		results: []model.ProfileEmbedding{
			{ID: 1, SourceDoc: "ref:skills", Term: "some irrelevant text", Weight: 0.1},
		},
	}
	embedder := &stubEmbedder{vec: []float32{0.1, 0.2}}

	svc := augment.New(repo, embedder, defaults())

	input := port.AugmentInput{
		ResumeText: "Original text",
		JDKeywords: []string{"relevant keyword"},
	}
	augmented, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(augmented, "some irrelevant text") {
		t.Error("expected low-similarity chunk to be filtered out")
	}
}

func TestAugmentResumeText_PassesRefDataThrough(t *testing.T) {
	repo := &stubProfileRepo{results: []model.ProfileEmbedding{}}
	embedder := &stubEmbedder{vec: []float32{0.1}}

	svc := augment.New(repo, embedder, defaults())

	existing := &port.ReferenceData{AllSkills: []string{"Go", "Kubernetes"}}
	input := port.AugmentInput{
		ResumeText: "text",
		RefData:    existing,
		JDKeywords: []string{},
	}
	_, refData, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refData == nil || len(refData.AllSkills) != 2 {
		t.Errorf("expected existing refData passed through, got %v", refData)
	}
}
