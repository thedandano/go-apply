package port

import "context"

type ChatMessage struct {
	Role    string
	Content string
}

type ChatOptions struct {
	Temperature float64
	MaxTokens   int
}

// LLMClient is the generic interface for any LLM provider.
// The default implementation uses the OpenAI-compatible chat completions protocol.
type LLMClient interface {
	ChatComplete(ctx context.Context, messages []ChatMessage, opts ChatOptions) (string, error)
}
