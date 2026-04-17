package tailor

import (
	"context"
	"log/slog"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Tailor = (*Service)(nil)

// Service implements port.Tailor via a two-tier cascade:
// tier-1 injects missing JD keywords into the Skills section;
// tier-2 rewrites relevant Experience bullets grounded in accomplishments.
type Service struct {
	llm      port.LLMClient
	defaults *config.AppDefaults
	log      *slog.Logger
}

// New constructs a Service with the provided dependencies.
func New(llm port.LLMClient, defaults *config.AppDefaults, log *slog.Logger) *Service {
	return &Service{
		llm:      llm,
		defaults: defaults,
		log:      log,
	}
}

// TailorResume executes the two-tier tailoring cascade against input.
// Tier-1 (keyword injection) always runs.
// Tier-2 (bullet rewriting) runs only when AccomplishmentsText is non-empty
// and Options.MaxTier2BulletRewrites > 0.
// A tier-2 LLM error degrades gracefully to the tier-1 result; no error is returned.
func (s *Service) TailorResume(ctx context.Context, input *model.TailorInput) (model.TailorResult, error) {
	allKeywords := append(input.JD.Required, input.JD.Preferred...) //nolint:gocritic // fresh slice intentional

	// Tier-1: inject missing keywords into Skills section.
	s.log.DebugContext(ctx, "tailor tier-1 start", "input_bytes", len(input.ResumeText), "keywords", len(allKeywords))
	tier1Text, addedKeywords := AddKeywordsToSkillsSection(input.ResumeText, allKeywords)
	s.log.DebugContext(ctx, "tailor tier-1 end", "output_bytes", len(tier1Text), "added_keywords", len(addedKeywords))

	result := model.TailorResult{
		ResumeLabel:   input.Resume.Label,
		TierApplied:   model.TierKeyword,
		AddedKeywords: addedKeywords,
		TailoredText:  tier1Text,
	}

	// Tier-2: rewrite relevant bullets when accomplishments and budget are provided.
	runTier2 := input.AccomplishmentsText != "" && input.Options.MaxTier2BulletRewrites > 0
	if !runTier2 {
		reason := "no accomplishments text"
		if input.AccomplishmentsText != "" {
			reason = "budget=0"
		}
		logger.Decision(ctx, s.log, "tailor.tier", "t1", reason)
		return result, nil
	}

	tier2Input := &BulletRewriteInput{
		Ctx:                 ctx,
		LLM:                 s.llm,
		Log:                 s.log,
		ResumeText:          tier1Text,
		JDKeywords:          allKeywords,
		AccomplishmentsText: input.AccomplishmentsText,
		Defaults:            s.defaults,
		MaxRewrites:         input.Options.MaxTier2BulletRewrites,
	}

	// rewriteBullets handles per-bullet LLM errors internally (log + skip).
	// It always returns nil error; degradation to tier-1 is implicit when no
	// changes are produced (e.g. all bullets fail or none match keywords).
	tier2Text, changes, _ := rewriteBullets(tier2Input)

	// If tier-2 produced changes, upgrade the result.
	if len(changes) > 0 {
		logger.Decision(ctx, s.log, "tailor.tier", "t2", "bullets rewritten", slog.Int("changes", len(changes)))
		result.TierApplied = model.TierBullet
		result.RewrittenBullets = changes
		result.TailoredText = tier2Text
	} else {
		logger.Decision(ctx, s.log, "tailor.tier", "t1", "no bullets rewritten")
	}

	return result, nil
}
