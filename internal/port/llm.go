package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// LLMClient is the generic interface for any LLM provider.
// The default implementation uses the OpenAI-compatible chat completions protocol.
type LLMClient interface {
	ChatComplete(ctx context.Context, messages []model.ChatMessage, opts model.ChatOptions) (string, error)
}
