package tailor

import (
	"context"
	"fmt"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/text"
)

// BulletRewriteInput holds the inputs for a tier-2 bullet rewrite call.
type BulletRewriteInput struct {
	ResumeText          string
	AccomplishmentsText string
	JDKeywords          []string
}

// RewriteBullets uses the LLM to rewrite experience bullets from input.ResumeText,
// incorporating input.JDKeywords and grounding rewrites in input.AccomplishmentsText.
// Returns the rewritten bullets (lines starting with "-"), capped at
// defaults.Tailor.MaxTier2BulletRewrites.
func RewriteBullets(
	ctx context.Context,
	llm port.LLMClient,
	defaults *config.AppDefaults,
	input BulletRewriteInput,
) ([]string, error) {
	if input.AccomplishmentsText == "" {
		return nil, fmt.Errorf("accomplishmentsText is required for tier-2 bullet rewrites")
	}

	bullets := text.ExtractExperienceBullets(input.ResumeText)
	if len(bullets) == 0 {
		return nil, nil
	}

	bulletList := make([]string, 0, len(bullets))
	for _, b := range bullets {
		bulletList = append(bulletList, "- "+b)
	}

	keywords := strings.Join(input.JDKeywords, ", ")
	if keywords == "" {
		keywords = "(none)"
	}

	systemPrompt := `You are a professional resume writer. Rewrite the provided experience bullets to better match the target job description keywords. Use ONLY facts from the accomplishments document — do not invent new achievements or numbers. Each rewritten bullet must start with "- ". Return ONLY the rewritten bullets, one per line, no other text.`

	userContent := fmt.Sprintf(
		"Job description keywords to incorporate: %s\n\nAccomplishments document (source of truth):\n%s\n\nOriginal bullets to rewrite:\n%s",
		keywords,
		input.AccomplishmentsText,
		strings.Join(bulletList, "\n"),
	)

	messages := []port.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	opts := port.ChatOptions{
		Temperature: defaults.LLM.BulletRewriteTemp,
		MaxTokens:   defaults.LLM.BulletRewriteMaxTokens,
	}

	resp, err := llm.ChatComplete(ctx, messages, opts)
	if err != nil {
		return nil, fmt.Errorf("bullet rewrite LLM call: %w", err)
	}

	var rewritten []string
	for _, line := range strings.Split(resp, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			content := strings.TrimPrefix(trimmed, "- ")
			if content != "" {
				rewritten = append(rewritten, content)
			}
		}
	}

	limit := defaults.Tailor.MaxTier2BulletRewrites
	if limit > 0 && len(rewritten) > limit {
		rewritten = rewritten[:limit]
	}

	return rewritten, nil
}
