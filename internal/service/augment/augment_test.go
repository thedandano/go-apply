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
	vector    []float32
	err       error
	callCount int
}

var _ port.EmbeddingClient = (*stubEmbeddingClient)(nil)

func (s *stubEmbeddingClient) Embed(_ context.Context, _ string) ([]float32, error) {
	s.callCount++
	return s.vector, s.err
}

// stubKeywordCacheRepository is a no-op keyword cache for unit tests.
type stubKeywordCacheRepository struct {
	stored   map[string][]float32
	getCalls int
	setCalls int
	getErr   error
	setErr   error
}

var _ port.KeywordCacheRepository = (*stubKeywordCacheRepository)(nil)

func newStubCache() *stubKeywordCacheRepository {
	return &stubKeywordCacheRepository{stored: make(map[string][]float32)}
}

func (s *stubKeywordCacheRepository) GetVector(_ context.Context, keyword string) ([]float32, bool, error) {
	s.getCalls++
	if s.getErr != nil {
		return nil, false, s.getErr
	}
	v, ok := s.stored[keyword]
	return v, ok, nil
}

func (s *stubKeywordCacheRepository) SetVector(_ context.Context, keyword string, vector []float32) error {
	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.stored[keyword] = vector
	return nil
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
		newStubCache(),
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
		newStubCache(),
		&stubEmbeddingClient{err: errors.New("embedding service unavailable")},
		testDefaults(),
		testLogger(),
	)

	input := port.AugmentInput{
		ResumeText: "experienced software engineer",
		RefData:    nil,
		JDKeywords: []string{"golang", "kubernetes"},
	}

	// Embedding failure is a degraded-mode path: augmentation is best-effort enrichment,
	// not a hard requirement. Returning the original text lets the apply pipeline continue
	// without augmentation rather than aborting the entire job application run.
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
		newStubCache(),
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
		newStubCache(),
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
		newStubCache(),
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
		newStubCache(),
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

func TestAugmentResumeText_UsesCachedVector(t *testing.T) {
	t.Parallel()

	// Pre-populate cache with the expected joined keyword key.
	cache := newStubCache()
	cachedVector := []float32{0.9, 0.8, 0.7}
	_ = cache.SetVector(context.Background(), "golang kubernetes", cachedVector)
	// Reset setCalls after seeding so we can verify no new SetVector is called.
	cache.setCalls = 0

	embedder := &stubEmbeddingClient{vector: fakeVector()}

	aboveThreshold := model.ProfileEmbedding{
		ID: 1, SourceDoc: "resume:backend", Term: "Backend systems work", Weight: 0.9,
	}
	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThreshold}},
		cache,
		embedder,
		testDefaults(),
		testLogger(),
	)

	input := port.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"golang", "kubernetes"},
	}

	got, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if embedder.callCount != 0 {
		t.Errorf("expected no embedding API call on cache hit, got %d calls", embedder.callCount)
	}
	if !strings.Contains(got, "Backend systems work") {
		t.Errorf("expected augmented text with cached vector result, got: %s", got)
	}
}

func TestAugmentResumeText_StoresMissInCache(t *testing.T) {
	t.Parallel()

	cache := newStubCache() // empty cache → miss
	embedder := &stubEmbeddingClient{vector: fakeVector()}

	aboveThreshold := model.ProfileEmbedding{
		ID: 1, SourceDoc: "resume:backend", Term: "Backend systems work", Weight: 0.9,
	}
	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThreshold}},
		cache,
		embedder,
		testDefaults(),
		testLogger(),
	)

	input := port.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"golang", "kubernetes"},
	}

	_, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if embedder.callCount != 1 {
		t.Errorf("expected exactly 1 embedding API call on cache miss, got %d", embedder.callCount)
	}
	if cache.setCalls != 1 {
		t.Errorf("expected SetVector to be called once for cache miss, got %d calls", cache.setCalls)
	}
	stored, ok, _ := cache.GetVector(context.Background(), "golang kubernetes")
	if !ok {
		t.Error("expected vector to be stored in cache after miss")
	}
	if len(stored) == 0 {
		t.Error("expected non-empty stored vector")
	}
}
