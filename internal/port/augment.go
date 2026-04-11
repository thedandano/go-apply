package port

import "context"

// Augmenter enriches resume text with semantically similar profile document chunks.
// The concrete implementation (service/augment.AugmentService) composes a
// ProfileRepository and an EmbeddingClient — both swappable via their own port interfaces.
// Using a port interface here (rather than *augment.AugmentService directly) allows
// tests to pass stub implementations to ApplyPipeline and TailorPipeline.
type Augmenter interface {
	// AugmentResumeText embeds JD keywords, finds similar profile docs,
	// and appends the most relevant chunks to resumeText.
	// Returns the augmented text and an optionally populated ReferenceData.
	AugmentResumeText(ctx context.Context, resumeText string, refData *ReferenceData, jdKeywords []string) (string, *ReferenceData, error)
}
