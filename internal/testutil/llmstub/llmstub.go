// Package llmstub provides a deterministic port.LLMClient for unit tests.
package llmstub

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Stub is a deterministic LLMClient for unit tests.
// It returns canned responses keyed by the last user message content,
// and can inject an error on a specific call index.
type Stub struct {
	responses map[string]string // message content → JSON response
	errOnCall int               // if > 0, return errOnCallErr on this call (1-based)
	errMsg    string
	callCount atomic.Int32
}

// New returns a Stub that returns responses[msgContent] for each ChatComplete call.
// errOnCall is 1-based (1 = fail on first call). Use 0 to never inject an error.
// errMsg is the error message returned when the call index matches errOnCall.
func New(responses map[string]string, errOnCall int, errMsg string) port.LLMClient {
	return &Stub{responses: responses, errOnCall: errOnCall, errMsg: errMsg}
}

func (s *Stub) ChatComplete(_ context.Context, messages []model.ChatMessage, _ model.ChatOptions) (string, error) {
	n := int(s.callCount.Add(1))
	if s.errOnCall > 0 && n == s.errOnCall {
		if s.errMsg != "" {
			return "", errors.New(s.errMsg)
		}
		return "", errors.New("stub: injected LLM error")
	}
	// Use the last user message as the lookup key.
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			if resp, ok := s.responses[messages[i].Content]; ok {
				return resp, nil
			}
		}
	}
	// Default: return empty JSON array (zero tags — valid, not an error).
	return "[]", nil
}
