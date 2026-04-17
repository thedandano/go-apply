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

	// Determine retrieval mode: "vector" if any suggestion has Similarity > 0,
	// "keyword_fallback" if all have Similarity == 0, "none" if empty.
	retrievalMode := "none"
	for _, chunks := range suggestions {
		for _, c := range chunks {
			if c.Similarity > 0 {
				retrievalMode = "vector"
				goto done
			}
		}
	}
	if len(suggestions) > 0 {
		retrievalMode = "keyword_fallback"
	}
done:
	return suggestions, retrievalMode, nil
}
