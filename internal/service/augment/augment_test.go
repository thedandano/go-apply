package augment_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/augment"
)

// --- stubs ---

// stubProfileRepository is a no-op profile repository for unit tests.
type stubProfileRepository struct {
	results []model.ProfileEmbedding
	err     error
}

var _ port.ProfileRepository = (*stubProfileRepository)(nil)

func (s *stubProfileRepository) UpsertDocument(_ context.Context, _ string, _ string, _ []float32) error {
	return nil
}

func (s *stubProfileRepository) FindSimilar(_ context.Context, _ []float32, _ int) ([]model.ProfileEmbedding, error) {
	return s.results, s.err
}

// stubEmbeddingClient returns a fixed vector or an error for unit tests.
type stubEmbeddingClient struct {
	vector []float32
	err    error
}

var _ port.EmbeddingClient = (*stubEmbeddingClient)(nil)

func (s *stubEmbeddingClient) Embed(_ context.Context, _ string) ([]float32, error) {
	return s.vector, s.err
}

// --- helpers ---

func testDefaults() *config.AppDefaults {
	d := config.EmbeddedDefaults()
	d.VectorSearch.SimilarityThreshold = 0.6
	d.VectorSearch.TopK = 5
	return d
}

func testLogger() *slog.Logger {
	return slog.Default()
}

func fakeVector() []float32 {
	return []float32{0.1, 0.2, 0.3}
}

// --- tests ---

func TestAugmentResumeText_ReturnsOriginalWhenKeywordsEmpty(t *testing.T) {
	t.Parallel()

	svc := augment.New(
		&stubProfileRepository{},
		&stubEmbeddingClient{vector: fakeVector()},
		testDefaults(),
		testLogger(),
	)

	input := port.AugmentInput{
		ResumeText: "experienced software engineer",
		RefData:    nil,
		JDKeywords: nil,
	}

	got, refData, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got != input.ResumeText {
		t.Errorf("expected original text %q, got %q", input.ResumeText, got)
	}
	if refData != input.RefData {
		t.Errorf("expected ref data to be unchanged")
	}
}

func TestAugmentResumeText_ReturnsOriginalWhenEmbeddingFails(t *testing.T) {
	t.Parallel()

	svc := augment.New(
		&stubProfileRepository{},
		&stubEmbeddingClient{err: errors.New("embedding service unavailable")},
		testDefaults(),
		testLogger(),
	)

	input := port.AugmentInput{
		ResumeText: "experienced software engineer",
		RefData:    nil,
		JDKeywords: []string{"golang", "kubernetes"},
	}

	got, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected graceful degradation (no error), got: %v", err)
	}
	if got != input.ResumeText {
		t.Errorf("expected original text on embedding failure, got %q", got)
	}
}

func TestAugmentResumeText_ReturnsOriginalWhenNoSimilarDocs(t *testing.T) {
	t.Parallel()

	svc := augment.New(
		&stubProfileRepository{results: nil, err: nil},
		&stubEmbeddingClient{vector: fakeVector()},
		testDefaults(),
		testLogger(),
	)

	input := port.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"golang", "distributed systems"},
	}

	got, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got != input.ResumeText {
		t.Errorf("expected original text when no similar docs, got %q", got)
	}
}

func TestAugmentResumeText_AppendsMatchingDocsAboveThreshold(t *testing.T) {
	t.Parallel()

	aboveThreshold := model.ProfileEmbedding{
		ID:        1,
		SourceDoc: "resume:backend",
		Term:      "Led distributed systems redesign",
		Weight:    0.85,
	}

	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThreshold}},
		&stubEmbeddingClient{vector: fakeVector()},
		testDefaults(),
		testLogger(),
	)

	input := port.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"distributed", "systems"},
	}

	got, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(got, "Led distributed systems redesign") {
		t.Errorf("expected augmented text to contain matching doc, got:\n%s", got)
	}
	if !strings.Contains(got, "--- Profile Reference ---") {
		t.Errorf("expected profile reference header in output, got:\n%s", got)
	}
}

func TestAugmentResumeText_ReturnsOriginalWhenFindSimilarFails(t *testing.T) {
	t.Parallel()

	svc := augment.New(
		&stubProfileRepository{err: errors.New("db unavailable")},
		&stubEmbeddingClient{vector: fakeVector()},
		testDefaults(),
		testLogger(),
	)

	input := port.AugmentInput{
		ResumeText: "experienced software engineer",
		RefData:    nil,
		JDKeywords: []string{"golang", "kubernetes"},
	}

	got, refData, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected graceful degradation (no error), got: %v", err)
	}
	if got != input.ResumeText {
		t.Errorf("expected original text on FindSimilar failure, got %q", got)
	}
	if refData != input.RefData {
		t.Errorf("expected ref data to be unchanged on FindSimilar failure")
	}
}

func TestAugmentResumeText_FiltersOutDocsBelowThreshold(t *testing.T) {
	t.Parallel()

	belowThreshold := model.ProfileEmbedding{
		ID:        2,
		SourceDoc: "resume:other",
		Term:      "Irrelevant content",
		Weight:    0.3, // below default threshold of 0.6
	}

	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{belowThreshold}},
		&stubEmbeddingClient{vector: fakeVector()},
		testDefaults(),
		testLogger(),
	)

	input := port.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"golang", "microservices"},
	}

	got, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got != input.ResumeText {
		t.Errorf("expected original text when all candidates below threshold, got %q", got)
	}
	if strings.Contains(got, "Irrelevant content") {
		t.Errorf("expected below-threshold doc to be filtered out")
	}
}
