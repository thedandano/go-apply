package orchestrator

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// ParseSections is not yet implemented — returns ErrNotSupportedInMCPMode until T019 lands.
func (o *LLMOrchestrator) ParseSections(_ context.Context, _ string) (model.SectionMap, error) {
	return model.SectionMap{}, model.ErrNotSupportedInMCPMode
}
