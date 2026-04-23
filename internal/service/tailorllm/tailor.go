package tailorllm

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/thedandano/go-apply/internal/model"
)

// TailorStore is the session-store interface required by LLMTailor.
// *mcpserver.TailorSessionStore satisfies this interface.
type TailorStore interface {
	Open(bundle string, input *model.TailorInput, timeout time.Duration) (string, error)
	Wait(ctx context.Context, id string) (model.TailorResult, error)
}

// Config holds LLMTailor configuration.
type Config struct {
	Timeout    time.Duration
	PromptBody string
}

// LLMTailor implements port.Tailor by delegating tailoring to the LLM
// via an async session store (open → wait for LLM submit).
type LLMTailor struct {
	cfg   Config
	store TailorStore
}

// New creates a new LLMTailor.
func New(cfg Config, store TailorStore) *LLMTailor {
	return &LLMTailor{cfg: cfg, store: store}
}

// TailorResume builds a prompt bundle, opens a session, and blocks until the
// LLM submits the tailored result or the session expires.
func (t *LLMTailor) TailorResume(ctx context.Context, input *model.TailorInput) (model.TailorResult, error) {
	bundle := t.buildBundle(input)
	sessionID, err := t.store.Open(bundle, input, t.cfg.Timeout)
	if err != nil {
		return model.TailorResult{}, fmt.Errorf("tailor: open session: %w", err)
	}
	result, err := t.store.Wait(ctx, sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "expired") {
			slog.InfoContext(ctx, "tailor_timeout",
				slog.String("session_id", sessionID),
			)
		} else {
			slog.InfoContext(ctx, "tailor_error",
				slog.String("session_id", sessionID),
				slog.Any("error", err),
			)
		}
		return model.TailorResult{}, err // preserve sentinel errors unchanged
	}
	return result, nil
}

func (t *LLMTailor) buildBundle(input *model.TailorInput) string {
	var sb strings.Builder
	sb.WriteString(t.cfg.PromptBody)
	if input.ResumeText != "" {
		sb.WriteString("\n\n## Resume\n\n")
		sb.WriteString(input.ResumeText)
	}
	if input.AccomplishmentsText != "" {
		sb.WriteString("\n\n## Accomplishments\n\n")
		sb.WriteString(input.AccomplishmentsText)
	}
	return sb.String()
}
