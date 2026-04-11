package port

import "context"

// EmbeddingClient produces dense vector representations of text.
// Decoupled from LLMClient intentionally — embedding models are often
// smaller, cheaper, or local (e.g. nomic-embed, mxbai-embed).
// The default implementation uses the same OpenAI-compatible /embeddings endpoint.
type EmbeddingClient interface {
	// Embed returns a float32 vector for the given text.
	Embed(ctx context.Context, text string) ([]float32, error)
}
