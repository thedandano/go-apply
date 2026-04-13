// Package augment enriches resume text by finding semantically similar profile
// document chunks and appending the most relevant ones.
package augment

import (
	"context"
	"log/slog"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Augmenter = (*Service)(nil)

const profileHeader = "\n\n--- Profile Reference ---\n"

// Service composes ProfileRepository, KeywordCacheRepository, and EmbeddingClient.
// It embeds JD keywords, retrieves similar profile document chunks,
// filters by similarity threshold, and appends relevant chunks to the resume text.
type Service struct {
	profile  port.ProfileRepository
	cache    port.KeywordCacheRepository
	embedder port.EmbeddingClient
	defaults *config.AppDefaults
	log      *slog.Logger
}

// New constructs a Service with the provided dependencies.
func New(profile port.ProfileRepository, cache port.KeywordCacheRepository, embedder port.EmbeddingClient, defaults *config.AppDefaults, log *slog.Logger) *Service {
	return &Service{
		profile:  profile,
		cache:    cache,
		embedder: embedder,
		defaults: defaults,
		log:      log,
	}
}

// AugmentResumeText embeds JD keywords, finds similar profile document chunks,
// and appends matching chunks above the similarity threshold to the resume text.
// Failures in embedding or similarity search degrade gracefully — the original
// text is returned without error so the pipeline can continue.
func (s *Service) AugmentResumeText(ctx context.Context, input port.AugmentInput) (string, *port.ReferenceData, error) {
	inputWords := len(strings.Fields(input.ResumeText))
	s.log.DebugContext(ctx, "augment started", "input_words", inputWords, "keyword_count", len(input.JDKeywords))

	if len(input.JDKeywords) == 0 || s.embedder == nil {
		s.log.DebugContext(ctx, "augment skipped: no keywords or no embedder")
		return input.ResumeText, input.RefData, nil
	}

	vector, err := s.embedWithCache(ctx, input.JDKeywords)
	if err != nil {
		s.log.WarnContext(ctx, "augment: embedding failed — returning original text", "error", err)
		return input.ResumeText, input.RefData, nil
	}

	docs, err := s.findRelevantDocs(ctx, vector)
	if err != nil {
		s.log.WarnContext(ctx, "augment: similarity search failed — returning original text", "error", err)
		return input.ResumeText, input.RefData, nil
	}
	if len(docs) == 0 {
		s.log.WarnContext(ctx, "augment: no relevant docs found — returning original text")
		return input.ResumeText, input.RefData, nil
	}

	augmented := appendDocs(input.ResumeText, docs)
	outputWords := len(strings.Fields(augmented))
	s.log.DebugContext(ctx, "augment complete",
		"input_words", inputWords,
		"output_words", outputWords,
		"chunks_added", len(docs),
	)

	return augmented, input.RefData, nil
}

// embedWithCache joins keywords into a single query string, checks the cache for a
// pre-computed embedding vector, and falls back to calling the embedding API on a miss.
// On a cache miss the resulting vector is stored in the cache; SetVector failures are
// logged as warnings but do not abort the pipeline.
func (s *Service) embedWithCache(ctx context.Context, keywords []string) ([]float32, error) {
	cacheKey := strings.Join(keywords, " ")

	if s.cache != nil {
		if cached, ok, err := s.cache.GetVector(ctx, cacheKey); err == nil && ok {
			s.log.DebugContext(ctx, "augment: keyword vector cache hit", "key_len", len(cacheKey))
			return cached, nil
		}
	}

	vector, err := s.embedder.Embed(ctx, cacheKey)
	if err != nil {
		return nil, err
	}

	if s.cache != nil {
		if setErr := s.cache.SetVector(ctx, cacheKey, vector); setErr != nil {
			s.log.WarnContext(ctx, "augment: failed to store vector in cache — continuing", "error", setErr)
		}
	}

	return vector, nil
}

// findRelevantDocs retrieves the top-k similar profile document chunks and filters
// them to those at or above the configured similarity threshold.
func (s *Service) findRelevantDocs(ctx context.Context, vector []float32) ([]model.ProfileEmbedding, error) {
	candidates, err := s.profile.FindSimilar(ctx, vector, s.defaults.VectorSearch.TopK)
	if err != nil {
		return nil, err
	}

	threshold := s.defaults.VectorSearch.SimilarityThreshold
	var docs []model.ProfileEmbedding
	for _, c := range candidates {
		if c.Weight >= threshold {
			docs = append(docs, c)
		}
	}

	if len(docs) == 0 && len(candidates) > 0 {
		s.log.WarnContext(ctx, "augment: all candidates below similarity threshold",
			"threshold", threshold,
			"candidates", len(candidates),
		)
	}

	return docs, nil
}

// appendDocs appends the matching profile document chunks under a header section.
func appendDocs(resumeText string, docs []model.ProfileEmbedding) string {
	chunks := make([]string, len(docs))
	for i, d := range docs {
		chunks[i] = d.Term
	}
	return resumeText + profileHeader + strings.Join(chunks, "\n")
}
