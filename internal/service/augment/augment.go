// Package augment enriches resume text by finding semantically similar profile
// document chunks and appending the most relevant ones.
package augment

import (
	"context"
	"log/slog"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Augmenter = (*Service)(nil)

const profileHeader = "\n\n--- Profile Reference ---\n"

// Service composes ProfileRepository and EmbeddingClient.
// It embeds JD keywords, retrieves similar profile document chunks,
// filters by similarity threshold, and appends relevant chunks to the resume text.
type Service struct {
	profile  port.ProfileRepository
	embedder port.EmbeddingClient
	defaults *config.AppDefaults
	log      *slog.Logger
}

// New constructs a Service with the provided dependencies.
func New(profile port.ProfileRepository, embedder port.EmbeddingClient, defaults *config.AppDefaults, log *slog.Logger) *Service {
	return &Service{
		profile:  profile,
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

	query := strings.Join(input.JDKeywords, " ")
	vector, err := s.embedder.Embed(ctx, query)
	if err != nil {
		s.log.WarnContext(ctx, "augment: embedding failed — returning original text", "error", err)
		return input.ResumeText, input.RefData, nil
	}

	candidates, err := s.profile.FindSimilar(ctx, vector, s.defaults.VectorSearch.TopK)
	if err != nil {
		s.log.WarnContext(ctx, "augment: similarity search failed — returning original text", "error", err)
		return input.ResumeText, input.RefData, nil
	}
	if len(candidates) == 0 {
		s.log.WarnContext(ctx, "augment: no similar docs found — returning original text")
		return input.ResumeText, input.RefData, nil
	}

	threshold := s.defaults.VectorSearch.SimilarityThreshold
	var chunks []string
	for _, c := range candidates {
		if c.Weight >= threshold {
			chunks = append(chunks, c.Term)
		}
	}

	if len(chunks) == 0 {
		s.log.WarnContext(ctx, "augment: all candidates below similarity threshold — returning original text",
			"threshold", threshold,
			"candidates", len(candidates),
		)
		return input.ResumeText, input.RefData, nil
	}

	augmented := input.ResumeText + profileHeader + strings.Join(chunks, "\n")
	outputWords := len(strings.Fields(augmented))
	s.log.DebugContext(ctx, "augment complete",
		"input_words", inputWords,
		"output_words", outputWords,
		"chunks_added", len(chunks),
	)

	return augmented, input.RefData, nil
}
