package tailor

import (
	"context"
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

		rewrittenContents, err := RewriteBullets(ctx, s.llm, s.defaults, BulletRewriteInput{
				ResumeText:          tailoredText,
				AccomplishmentsText: input.AccomplishmentsText,
				JDKeywords:          jdKeywords,
			})
		if err != nil {
			// Tier-2 failed — keep tier-1 result and return without error.
			result.TailoredText = tailoredText
			return result, nil
		}

		if len(rewrittenContents) > 0 {
			originalBullets := text.ExtractExperienceBullets(tailoredText)

			changes := make([]model.BulletChange, 0, len(rewrittenContents))
			for i, rewritten := range rewrittenContents {
				var original string
				if i < len(originalBullets) {
					original = originalBullets[i]
				}
				change := model.BulletChange{
					Original:  original,
					Rewritten: rewritten,
				}
				if original != "" {
					var replaced bool
					tailoredText, replaced = replaceBulletInText(tailoredText, original, rewritten)
					if !replaced {
						change.Original = ""
					}
				}
				changes = append(changes, change)
			}
			result.RewrittenBullets = changes
			result.TierApplied = model.TierBullet
		}
	}

	result.TailoredText = tailoredText
	return result, nil
}

// replaceBulletInText finds the line containing the original bullet content
// (stripped of its marker) and replaces that whole line with the rewritten bullet,
// preserving the original bullet marker prefix.
// Returns the modified text and true if a replacement was made.
func replaceBulletInText(text, original, rewritten string) (string, bool) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		// Extract marker prefix (any leading bullet chars + spaces).
		trimmed := strings.TrimLeft(line, "•-*–▸○ \t")
		if strings.EqualFold(strings.TrimSpace(trimmed), strings.TrimSpace(original)) {
			// Preserve original marker: everything before the content.
			prefix := line[:len(line)-len(trimmed)]
			// Strip any "- " prefix the LLM added to rewritten.
			rewrittenContent := strings.TrimPrefix(strings.TrimPrefix(rewritten, "- "), "-")
			rewrittenContent = strings.TrimSpace(rewrittenContent)
			lines[i] = prefix + rewrittenContent
			return strings.Join(lines, "\n"), true
		}
	}
	return text, false
}
