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

type stubProfileRepository struct {
	results []model.ProfileEmbedding
	findErr error
}

var _ port.ProfileRepository = (*stubProfileRepository)(nil)

func (s *stubProfileRepository) UpsertDocument(_ context.Context, _ string, _ string, _ []float32) error {
	return nil
}

func (s *stubProfileRepository) FindSimilar(_ context.Context, _ []float32, _ int) ([]model.ProfileEmbedding, error) {
	return s.results, s.findErr
}

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

type stubLLMClient struct {
	response string
	err      error
	calls    int
	lastMsgs []model.ChatMessage
}

var _ port.LLMClient = (*stubLLMClient)(nil)

func (s *stubLLMClient) ChatComplete(_ context.Context, messages []model.ChatMessage, _ model.ChatOptions) (string, error) {
	s.calls++
	s.lastMsgs = messages
	return s.response, s.err
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

func aboveThresholdEmbedding() model.ProfileEmbedding {
	return model.ProfileEmbedding{
		ID:        1,
		SourceDoc: "resume:backend",
		Term:      "Led distributed systems redesign at scale",
		Weight:    0.85,
	}
}

// --- tests ---

func TestAugmentResumeText_SkipsWhenNoKeywords(t *testing.T) {
	t.Parallel()

	llm := &stubLLMClient{response: "augmented"}
	svc := augment.New(
		&stubProfileRepository{},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		llm,
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
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
	if llm.calls != 0 {
		t.Errorf("expected LLM never called when no keywords, got %d calls", llm.calls)
	}
}

func TestAugmentResumeText_VectorPath_IncorporatesChunksViaLLM(t *testing.T) {
	t.Parallel()

	llm := &stubLLMClient{response: "augmented resume"}
	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThresholdEmbedding()}},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		llm,
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"distributed", "systems"},
	}

	got, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if got != "augmented resume" {
		t.Errorf("expected LLM response as augmented text, got %q", got)
	}
	if llm.calls != 1 {
		t.Errorf("expected exactly 1 LLM call, got %d", llm.calls)
	}
}

func TestAugmentResumeText_VectorPath_ReturnsOriginalWhenNoChunksAboveThreshold(t *testing.T) {
	t.Parallel()

	belowThreshold := model.ProfileEmbedding{
		ID:        2,
		SourceDoc: "resume:other",
		Term:      "Irrelevant content",
		Weight:    0.3,
	}
	llm := &stubLLMClient{}
	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{belowThreshold}},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		llm,
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
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
	if llm.calls != 0 {
		t.Errorf("expected LLM never called when no chunks above threshold, got %d calls", llm.calls)
	}
}

func TestAugmentResumeText_VectorPath_ReturnsOriginalWhenFindSimilarFails(t *testing.T) {
	t.Parallel()

	// FindSimilar fails per-keyword (logged + skipped); no fallback — original resume returned.
	svc := augment.New(
		&stubProfileRepository{findErr: errors.New("db unavailable")},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		&stubLLMClient{},
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"golang", "kubernetes"},
	}

	got, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error when FindSimilar fails (per-keyword skip), got: %v", err)
	}
	if got != input.ResumeText {
		t.Errorf("expected original resume text when no chunks found, got: %q", got)
	}
}

func TestAugmentResumeText_VectorPath_ErrorWhenLLMFails(t *testing.T) {
	t.Parallel()

	llm := &stubLLMClient{err: errors.New("llm timeout")}
	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThresholdEmbedding()}},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		llm,
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"distributed", "systems"},
	}

	_, _, err := svc.AugmentResumeText(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when LLM fails")
	}
	if !strings.Contains(err.Error(), "incorporate") {
		t.Errorf("expected error to mention 'incorporate', got: %v", err)
	}
}

func TestAugmentResumeText_VectorPath_ErrorWhenLLMReturnsEmpty(t *testing.T) {
	t.Parallel()

	llm := &stubLLMClient{response: ""}
	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThresholdEmbedding()}},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		llm,
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"distributed", "systems"},
	}

	_, _, err := svc.AugmentResumeText(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when LLM returns empty response")
	}
}

func TestAugmentResumeText_EmbedderFails_ReturnsOriginalResume(t *testing.T) {
	t.Parallel()

	// Embedder fails — no vectors, no chunks, original resume returned. No fallback.
	embedder := &stubEmbeddingClient{err: errors.New("embedding service unavailable")}
	svc := augment.New(
		&stubProfileRepository{},
		newStubCache(),
		embedder,
		&stubLLMClient{},
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"golang"},
	}

	got, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error when embedder fails (per-keyword skip), got: %v", err)
	}
	if got != input.ResumeText {
		t.Errorf("expected original resume text when embedder fails, got: %q", got)
	}
	if embedder.callCount != 1 {
		t.Errorf("expected embedder called exactly once (tried, failed), got %d calls", embedder.callCount)
	}
}

func TestAugmentResumeText_LLMReceivesCorrectPromptStructure(t *testing.T) {
	t.Parallel()

	llm := &stubLLMClient{response: "augmented resume"}
	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThresholdEmbedding()}},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		llm,
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"distributed", "systems"},
	}

	_, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(llm.lastMsgs) != 2 {
		t.Fatalf("expected exactly 2 messages sent to LLM, got %d", len(llm.lastMsgs))
	}
	if llm.lastMsgs[0].Role != "system" {
		t.Errorf("expected first message role 'system', got %q", llm.lastMsgs[0].Role)
	}
	if !strings.Contains(llm.lastMsgs[0].Content, "fabricate") {
		t.Errorf("expected system message to contain 'fabricate', got: %q", llm.lastMsgs[0].Content)
	}
	if llm.lastMsgs[1].Role != "user" {
		t.Errorf("expected second message role 'user', got %q", llm.lastMsgs[1].Role)
	}
	if !strings.Contains(llm.lastMsgs[1].Content, input.ResumeText) {
		t.Errorf("expected user message to contain resume text")
	}
	if !strings.Contains(llm.lastMsgs[1].Content, "Led distributed systems redesign at scale") {
		t.Errorf("expected user message to contain chunk text")
	}
}

func TestAugmentResumeText_CacheHitSkipsEmbedder(t *testing.T) {
	t.Parallel()

	cache := newStubCache()
	cachedVector := []float32{0.9, 0.8, 0.7}
	_ = cache.SetVector(context.Background(), "golang", cachedVector)
	_ = cache.SetVector(context.Background(), "kubernetes", cachedVector)
	cache.setCalls = 0 // reset after seeding

	embedder := &stubEmbeddingClient{vector: fakeVector()}
	llm := &stubLLMClient{response: "augmented resume"}

	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThresholdEmbedding()}},
		cache,
		embedder,
		llm,
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"golang", "kubernetes"},
	}

	_, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if embedder.callCount != 0 {
		t.Errorf("expected no embedding API call on cache hit, got %d calls", embedder.callCount)
	}
	if llm.calls != 1 {
		t.Errorf("expected LLM called once with cached vector result, got %d calls", llm.calls)
	}
}

func TestAugmentResumeText_CacheMissStoresVector(t *testing.T) {
	t.Parallel()

	cache := newStubCache()
	embedder := &stubEmbeddingClient{vector: fakeVector()}
	llm := &stubLLMClient{response: "augmented resume"}

	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThresholdEmbedding()}},
		cache,
		embedder,
		llm,
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"golang", "kubernetes"},
	}

	_, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if embedder.callCount != 2 {
		t.Errorf("expected 2 embedding API calls on cache miss (one per keyword), got %d", embedder.callCount)
	}
	if cache.setCalls != 2 {
		t.Errorf("expected SetVector called twice for cache miss (one per keyword), got %d calls", cache.setCalls)
	}
}

func TestAugmentResumeText_CacheWriteFailureContinues(t *testing.T) {
	t.Parallel()

	cache := newStubCache()
	cache.setErr = errors.New("write failed")
	llm := &stubLLMClient{response: "augmented resume"}

	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThresholdEmbedding()}},
		cache,
		&stubEmbeddingClient{vector: fakeVector()},
		llm,
		testDefaults(),
		testLogger(),
	)

	input := model.AugmentInput{
		ResumeText: "experienced software engineer",
		JDKeywords: []string{"golang", "kubernetes"},
	}

	got, _, err := svc.AugmentResumeText(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error despite cache write failure, got: %v", err)
	}
	if got != "augmented resume" {
		t.Errorf("expected LLM response despite cache write failure, got %q", got)
	}
	if llm.calls != 1 {
		t.Errorf("expected LLM called once despite cache write failure, got %d calls", llm.calls)
	}
}

func TestNew_PanicsOnNilEmbedder(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when embedder is nil, but no panic occurred")
		}
	}()

	augment.New(
		&stubProfileRepository{},
		newStubCache(),
		nil, // nil embedder
		&stubLLMClient{},
		testDefaults(),
		testLogger(),
	)
}

// --- SuggestForKeywords tests ---

func TestSuggestForKeywords_ReturnsNilForEmptyKeywords(t *testing.T) {
	t.Parallel()

	svc := augment.New(
		&stubProfileRepository{},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		nil, // no LLM needed
		testDefaults(),
		testLogger(),
	)

	suggestions, err := svc.SuggestForKeywords(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if suggestions != nil {
		t.Errorf("expected nil suggestions for empty keywords, got: %v", suggestions)
	}
}

func TestSuggestForKeywords_VectorPath_ReturnsSuggestions(t *testing.T) {
	t.Parallel()

	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThresholdEmbedding()}},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		nil, // no LLM needed
		testDefaults(),
		testLogger(),
	)

	suggestions, err := svc.SuggestForKeywords(context.Background(), []string{"distributed"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(suggestions) == 0 {
		t.Fatal("expected suggestions, got none")
	}
	chunks, ok := suggestions["distributed"]
	if !ok || len(chunks) == 0 {
		t.Error("expected suggestions for keyword 'distributed'")
	}
	if chunks[0].Similarity == 0 {
		t.Error("expected non-zero similarity for vector match")
	}
	if chunks[0].Text == "" {
		t.Error("expected non-empty chunk text")
	}
}

func TestSuggestForKeywords_BelowThreshold_NoSuggestions(t *testing.T) {
	t.Parallel()

	below := model.ProfileEmbedding{
		ID: 1, SourceDoc: "resume:other", Term: "irrelevant", Weight: 0.1,
	}
	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{below}},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		nil,
		testDefaults(),
		testLogger(),
	)

	suggestions, err := svc.SuggestForKeywords(context.Background(), []string{"golang"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(suggestions["golang"]) != 0 {
		t.Errorf("expected no suggestions for keyword below threshold, got: %v", suggestions["golang"])
	}
}

func TestSuggestForKeywords_EmbedderFails_NoSuggestions(t *testing.T) {
	t.Parallel()

	// Embedder fails — no vectors produced, no suggestions returned. No fallback.
	embedder := &stubEmbeddingClient{err: errors.New("embedding unavailable")}
	svc := augment.New(
		&stubProfileRepository{},
		newStubCache(),
		embedder,
		nil,
		testDefaults(),
		testLogger(),
	)

	suggestions, err := svc.SuggestForKeywords(context.Background(), []string{"golang"})
	if err != nil {
		t.Fatalf("expected no error when embedder fails, got: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected nil/empty suggestions when embedder fails, got: %v", suggestions)
	}
}

func TestSuggestForKeywords_NoLLMCalled(t *testing.T) {
	t.Parallel()

	llm := &stubLLMClient{response: "should not be called"}
	svc := augment.New(
		&stubProfileRepository{results: []model.ProfileEmbedding{aboveThresholdEmbedding()}},
		newStubCache(),
		&stubEmbeddingClient{vector: fakeVector()},
		llm,
		testDefaults(),
		testLogger(),
	)

	_, err := svc.SuggestForKeywords(context.Background(), []string{"distributed"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if llm.calls != 0 {
		t.Errorf("SuggestForKeywords must never call LLM, got %d calls", llm.calls)
	}
}
