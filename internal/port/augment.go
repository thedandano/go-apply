package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// Augmenter enriches resume text with semantically similar profile document chunks.
// The concrete implementation (service/augment.Service) composes a ProfileRepository
// and an EmbeddingClient — both swappable via their own port interfaces.
// Using a port interface here (rather than *augment.Service directly) allows
// tests to pass stub implementations to the pipeline.
type Augmenter interface {
	// AugmentResumeText embeds JD keywords, finds similar profile docs,
	// and incorporates the most relevant chunks into resumeText via LLM.
	// Returns the augmented text and an optionally populated ReferenceData.
	AugmentResumeText(ctx context.Context, input model.AugmentInput) (string, *model.ReferenceData, error)
}
