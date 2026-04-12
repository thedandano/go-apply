package augment

import (
	"context"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
)

// Service enriches resume text by appending semantically relevant
// profile document chunks retrieved via vector similarity.
// Implements port.Augmenter.
type Service struct {
	repo     port.ProfileRepository
	embedder port.EmbeddingClient
	defaults *config.AppDefaults
}

var _ port.Augmenter = (*Service)(nil)

// New returns a Service wired with the given repository, embedding
// client, and application defaults.
func New(repo port.ProfileRepository, embedder port.EmbeddingClient, defaults *config.AppDefaults) *Service {
	return &Service{repo: repo, embedder: embedder, defaults: defaults}
}

// AugmentResumeText embeds the JD keywords, finds the most similar profile
// document chunks, filters by similarity threshold, and appends qualifying
// chunks to the resume text. Failures in embedding or repository lookup are
// handled gracefully — the original text is returned unmodified.
func (s *Service) AugmentResumeText(ctx context.Context, input port.AugmentInput) (string, *port.ReferenceData, error) {
	refData := input.RefData

	if len(input.JDKeywords) == 0 {
		return input.ResumeText, refData, nil
	}

	queryText := strings.Join(input.JDKeywords, " ")

	vec, err := s.embedder.Embed(ctx, queryText)
	if err != nil {
		// Degrade: return original text without augmentation.
		return input.ResumeText, refData, nil
	}

	topK := s.defaults.VectorSearch.TopK
	similar, err := s.repo.FindSimilar(ctx, vec, topK)
	if err != nil {
		// Degrade: return original text without augmentation.
		return input.ResumeText, refData, nil
	}

	threshold := s.defaults.VectorSearch.SimilarityThreshold

	var chunks []string
	for _, doc := range similar {
		if doc.Weight >= threshold {
			chunks = append(chunks, doc.Term)
		}
	}

	if len(chunks) == 0 {
		return input.ResumeText, refData, nil
	}

	augmented := input.ResumeText + "\n\n" + strings.Join(chunks, "\n")

	if refData == nil {
		refData = &port.ReferenceData{}
	}

	return augmented, refData, nil
}
