// Package augment enriches resume text by retrieving relevant profile document
// chunks (via vector similarity or keyword fallback) and incorporating them via LLM.
package augment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Augmenter = (*Service)(nil)

// retrievedChunk is an internal type representing a profile document chunk
// selected for incorporation into the resume, regardless of retrieval path.
type retrievedChunk struct {
	Source string
	Text   string
}

// Service composes ProfileRepository, KeywordCacheRepository, EmbeddingClient, and LLMClient.
// It retrieves relevant profile document chunks by vector similarity (with keyword fallback)
// and incorporates them into the resume text via LLM.
type Service struct {
	profile  port.ProfileRepository
	cache    port.KeywordCacheRepository
	embedder port.EmbeddingClient
	llm      port.LLMClient
	defaults *config.AppDefaults
	log      *slog.Logger
}

// New constructs a Service with the provided dependencies.
// Panics if embedder is nil — a nil embedder indicates a wiring error.
func New(profile port.ProfileRepository, cache port.KeywordCacheRepository, embedder port.EmbeddingClient, llm port.LLMClient, defaults *config.AppDefaults, log *slog.Logger) *Service {
	if embedder == nil {
		panic("augment.New: embedder must not be nil — wire a real EmbeddingClient or check pipeline initialization")
	}
	return &Service{
		profile:  profile,
		cache:    cache,
		embedder: embedder,
		llm:      llm,
		defaults: defaults,
		log:      log,
	}
}

// AugmentResumeText retrieves relevant profile document chunks and incorporates them
// into the resume text via LLM. Returns an error if retrieval or incorporation fails.
func (s *Service) AugmentResumeText(ctx context.Context, input model.AugmentInput) (string, *model.ReferenceData, error) {
	s.log.DebugContext(ctx, "augment started", "input_words", len(strings.Fields(input.ResumeText)), "keyword_count", len(input.JDKeywords))

	if len(input.JDKeywords) == 0 {
		s.log.DebugContext(ctx, "augment skipped: no keywords")
		return input.ResumeText, input.RefData, nil
	}

	chunks, err := s.retrieveChunks(ctx, input.JDKeywords)
	if err != nil {
		return "", nil, fmt.Errorf("augment: retrieve chunks: %w", err)
	}
	if len(chunks) == 0 {
		s.log.WarnContext(ctx, "augment: no relevant chunks found — returning original resume")
		return input.ResumeText, input.RefData, nil
	}

	augmented, err := s.incorporateChunks(ctx, input.ResumeText, chunks, input.JDKeywords)
	if err != nil {
		return "", nil, fmt.Errorf("augment: incorporate chunks: %w", err)
	}

	s.log.DebugContext(ctx, "augment complete", "output_words", len(strings.Fields(augmented)), "chunks_used", len(chunks))
	return augmented, input.RefData, nil
}

// retrieveChunks attempts vector retrieval first. On failure, falls back to keyword matching.
func (s *Service) retrieveChunks(ctx context.Context, keywords []string) ([]retrievedChunk, error) {
	chunks, err := s.retrieveByVector(ctx, keywords)
	if err != nil {
		s.log.WarnContext(ctx, "augment: vector retrieval failed — falling back to keyword matching", "error", err)
		return s.retrieveByKeyword(ctx, keywords)
	}
	return chunks, nil
}

// retrieveByVector embeds the keywords (with cache), then finds similar profile chunks.
func (s *Service) retrieveByVector(ctx context.Context, keywords []string) ([]retrievedChunk, error) {
	vector, stored, err := s.embedWithCache(ctx, keywords)
	if err != nil {
		return nil, fmt.Errorf("embed keywords: %w", err)
	}
	if !stored && s.cache != nil {
		cacheKey := strings.Join(keywords, " ")
		if setErr := s.cache.SetVector(ctx, cacheKey, vector); setErr != nil {
			s.log.WarnContext(ctx, "augment: cache write failed — continuing", "error", setErr)
		}
	}

	candidates, err := s.profile.FindSimilar(ctx, vector, s.defaults.VectorSearch.TopK)
	if err != nil {
		return nil, fmt.Errorf("find similar chunks: %w", err)
	}

	threshold := s.defaults.VectorSearch.SimilarityThreshold
	maxChunks := s.defaults.Augment.MaxChunksToIncorporate
	var chunks []retrievedChunk
	for _, c := range candidates {
		if c.Weight >= threshold {
			chunks = append(chunks, retrievedChunk{Source: c.SourceDoc, Text: c.Term})
			if len(chunks) >= maxChunks {
				break
			}
		}
	}
	if len(chunks) == 0 && len(candidates) > 0 {
		s.log.WarnContext(ctx, "augment: all vector candidates below threshold", "threshold", threshold, "candidates", len(candidates))
	}
	return chunks, nil
}

// retrieveByKeyword fetches all documents and filters by keyword match count.
func (s *Service) retrieveByKeyword(ctx context.Context, keywords []string) ([]retrievedChunk, error) {
	docs, err := s.profile.ListDocuments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list documents for keyword fallback: %w", err)
	}

	minCount := s.defaults.Augment.KeywordMatchMinCount
	maxChunks := s.defaults.Augment.MaxChunksToIncorporate
	var chunks []retrievedChunk
	for _, doc := range docs {
		lower := strings.ToLower(doc.Text)
		hits := 0
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				hits++
			}
		}
		if hits >= minCount {
			chunks = append(chunks, retrievedChunk{Source: doc.Source, Text: doc.Text})
			if len(chunks) >= maxChunks {
				break
			}
		}
	}
	s.log.DebugContext(ctx, "augment: keyword fallback complete", "total_docs", len(docs), "matched", len(chunks))
	return chunks, nil
}

// embedWithCache joins keywords into a single query string and checks the cache.
// Returns (vector, true, nil) on a cache hit, or (vector, false, nil) on a cache miss
// after calling the embedding API. The caller is responsible for storing misses in cache.
func (s *Service) embedWithCache(ctx context.Context, keywords []string) ([]float32, bool, error) {
	cacheKey := strings.Join(keywords, " ")

	if s.cache != nil {
		if cached, ok, err := s.cache.GetVector(ctx, cacheKey); err == nil && ok {
			s.log.DebugContext(ctx, "augment: keyword vector cache hit", "key_len", len(cacheKey))
			return cached, true, nil
		}
	}

	vector, err := s.embedder.Embed(ctx, cacheKey)
	if err != nil {
		return nil, false, err
	}

	return vector, false, nil
}

const incorporationSystemPrompt = `You are a resume augmentation assistant. Incorporate relevant experience from the provided profile chunks into the resume.

Rules:
1. Only use information present in the provided chunks. Never fabricate experience, metrics, or accomplishments.
2. Integrate chunk content naturally into the existing resume structure — do not append a separate section.
3. Strengthen existing bullet points with specific details from chunks where relevant.
4. Add bullet points under appropriate sections when chunks contain relevant experience not already represented.
5. Preserve the resume's existing formatting, tone, and structure.
6. Prioritise content that matches the provided job description keywords.
7. Return the complete augmented resume — not a diff, not just the additions.
8. If none of the chunks are relevant, return the original resume unchanged.`

const incorporationUserPromptFmt = `Resume to augment:

%s

Retrieved profile chunks:

%s

Job description keywords: %s

Return the complete augmented resume.`

// incorporateChunks calls the LLM to weave the retrieved chunks into the resume.
func (s *Service) incorporateChunks(ctx context.Context, resumeText string, chunks []retrievedChunk, keywords []string) (string, error) {
	var sb strings.Builder
	for i, c := range chunks {
		fmt.Fprintf(&sb, "Chunk %d [%s]:\n%s\n\n", i+1, c.Source, c.Text)
	}

	messages := []model.ChatMessage{
		{Role: "system", Content: incorporationSystemPrompt},
		{Role: "user", Content: fmt.Sprintf(incorporationUserPromptFmt, resumeText, sb.String(), strings.Join(keywords, ", "))},
	}

	result, err := s.llm.ChatComplete(ctx, messages, model.ChatOptions{
		Temperature: s.defaults.Augment.IncorporationTemp,
		MaxTokens:   s.defaults.Augment.IncorporationMaxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("LLM chat completion: %w", err)
	}
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("LLM returned empty response")
	}
	return strings.TrimSpace(result), nil
}
