package tailor

import (
	"context"
	"fmt"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/text"
)

// Compile-time interface satisfaction check.
var _ port.Tailor = (*Service)(nil)

// Service implements port.Tailor using a two-tier keyword/bullet cascade.
type Service struct {
	llm      port.LLMClient
	defaults *config.AppDefaults
}

// New constructs a tailor Service.
func New(llm port.LLMClient, defaults *config.AppDefaults) *Service {
	return &Service{llm: llm, defaults: defaults}
}

// TailorResume runs the two-tier cascade on the resume:
//  1. Tier 1: inject missing keywords into the skills section.
//  2. Tier 2 (optional): rewrite experience bullets via LLM.
func (s *Service) TailorResume(ctx context.Context, input port.TailorInput) (model.TailorResult, error) {
	result := model.TailorResult{
		ResumeLabel: input.Resume.Label,
		TierApplied: model.TierNone,
	}

	tailoredText := input.ResumeText

	// ── Tier 1: keyword injection ─────────────────────────────────────────────
	tier1Text, addedKWs := AddKeywordsToSkillsSection(tailoredText, input.ScoreBefore.Keywords.ReqUnmatched)
	if len(addedKWs) > 0 {
		tailoredText = tier1Text
		result.AddedKeywords = addedKWs
		result.TierApplied = model.TierKeyword
	}

	// ── Tier 2: bullet rewrites ───────────────────────────────────────────────
	if input.AccomplishmentsText != "" && input.Options.MaxTier2BulletRewrites > 0 {
		jdKeywords := make([]string, 0, len(input.ScoreBefore.Keywords.ReqUnmatched)+len(input.ScoreBefore.Keywords.PrefUnmatched))
		jdKeywords = append(jdKeywords, input.ScoreBefore.Keywords.ReqUnmatched...)
		jdKeywords = append(jdKeywords, input.ScoreBefore.Keywords.PrefUnmatched...)

		rewrittenContents, err := RewriteBullets(ctx, s.llm, s.defaults, tailoredText, input.AccomplishmentsText, jdKeywords)
		if err != nil {
			return result, fmt.Errorf("tier-2 bullet rewrite: %w", err)
		}

		if len(rewrittenContents) > 0 {
			originalBullets := text.ExtractExperienceBullets(tailoredText)

			changes := make([]model.BulletChange, 0, len(rewrittenContents))
			for i, rewritten := range rewrittenContents {
				var original string
				if i < len(originalBullets) {
					original = originalBullets[i]
				}
				changes = append(changes, model.BulletChange{
					Original:  original,
					Rewritten: rewritten,
				})
				// Replace original bullet in text with the rewritten one.
				if original != "" {
					tailoredText = strings.Replace(tailoredText, "- "+original, "- "+rewritten, 1)
				}
			}
			result.RewrittenBullets = changes
			result.TierApplied = model.TierBullet
		}
	}

	result.TailoredText = tailoredText
	return result, nil
}
