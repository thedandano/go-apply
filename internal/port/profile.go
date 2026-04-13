package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// ProfileRepository manages the vector store backed by sqlite-vec.
// All embedding vectors are stored as float32 blobs in the vec0 virtual table.
// Swap the implementation to use any other vector store (pgvector, chromem-go, etc.)
// by providing a new struct that satisfies this interface.
// CRUD only — augmentation logic lives in service/augment/AugmentService.
type ProfileRepository interface {
	// UpsertDocument stores or replaces the embedding vector for a named document chunk.
	// sourceDoc examples: "resume:backend", "ref:skills", "accomplishments"
	UpsertDocument(ctx context.Context, sourceDoc string, text string, vector []float32) error

	// FindSimilar returns the top-k document chunks most similar to the query vector.
	// Uses sqlite-vec's vec_distance_cosine under the hood.
	FindSimilar(ctx context.Context, queryVector []float32, k int) ([]model.ProfileEmbedding, error)

	// ListDocuments returns all stored document chunks for keyword-based fallback retrieval.
	ListDocuments(ctx context.Context) ([]model.ProfileDocument, error)
}
