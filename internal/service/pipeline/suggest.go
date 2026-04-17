package pipeline

import (
	"context"
	"fmt"

	"github.com/thedandano/go-apply/internal/model"
)

// SuggestTailoring retrieves profile chunks relevant to the JD's missing keywords
// from the profile bank and returns them grouped by keyword.
// This is a diagnostic method — it surfaces what the profile bank found, for
// orchestrator visibility. T1 and T2 retrieve internally; this result does not
// feed them.
func (p *ApplyPipeline) SuggestTailoring(ctx context.Context, jd *model.JDData, _ ScoreResumeResult) (model.TailorSuggestions, string, error) {
	if p.augment == nil {
		return nil, "none", nil
	}
	allKeywords := append(jd.Required, jd.Preferred...) //nolint:gocritic
	suggestions, err := p.augment.SuggestForKeywords(ctx, allKeywords)
	if err != nil {
		return nil, "none", fmt.Errorf("suggest tailoring: %w", err)
	}
	if suggestions == nil {
		return nil, "none", nil
	}

	// Retrieval is vector-only: mode is "vector" when suggestions were found, "none" otherwise.
	retrievalMode := "none"
	if len(suggestions) > 0 {
		retrievalMode = "vector"
	}
	return suggestions, retrievalMode, nil
}
