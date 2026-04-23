package tailorllm

import (
	"context"
	"errors"

	"github.com/thedandano/go-apply/internal/model"
)

// ErrHeadlessTailorNotImplemented is returned by HeadlessNullTailor to signal
// that LLM tailoring is not available in headless/CLI mode.
var ErrHeadlessTailorNotImplemented = errors.New("headless-mode tailor not implemented")

// HeadlessNullTailor is a port.Tailor implementation that always returns
// ErrHeadlessTailorNotImplemented. It is wired in CLI/headless mode where
// the agent-driven tailor flow (tailor_begin / tailor_submit) is unavailable.
type HeadlessNullTailor struct{}

// TailorResume always returns ErrHeadlessTailorNotImplemented.
func (h *HeadlessNullTailor) TailorResume(_ context.Context, _ *model.TailorInput) (model.TailorResult, error) {
	return model.TailorResult{}, ErrHeadlessTailorNotImplemented
}
