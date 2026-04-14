package tailor

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// bulletMarkers lists the recognized bullet markers at the start of a trimmed line.
var bulletMarkers = []string{"•", "-", "*"}

// experienceHeaderRe-like pattern handled inline for clarity.

// extractExperienceBullets returns all bullet lines from the Experience section.
// Bullet markers recognized: '•', '-', '*' at the start of the trimmed line.
func extractExperienceBullets(resumeText string) []string {
	lines := strings.Split(resumeText, "\n")

	experienceStart := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isExperienceHeader(trimmed) {
			experienceStart = i
			break
		}
	}

	if experienceStart == -1 {
		return nil
	}

	// Find end of Experience section: next section header or end of file.
	experienceEnd := len(lines)
	for i := experienceStart + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if isHeaderLine(trimmed) && !isExperienceHeader(trimmed) {
			experienceEnd = i
			break
		}
	}

	var bullets []string
	for _, line := range lines[experienceStart+1 : experienceEnd] {
		trimmed := strings.TrimSpace(line)
		if isBulletLine(trimmed) {
			bullets = append(bullets, line)
		}
	}
	return bullets
}

// isExperienceHeader returns true for common "Experience" section headers.
func isExperienceHeader(trimmed string) bool {
	lower := strings.ToLower(trimmed)
	// Strip markdown heading markers.
	lower = strings.TrimLeft(lower, "#")
	lower = strings.TrimSpace(lower)
	lower = strings.TrimRight(lower, ":")
	lower = strings.TrimSpace(lower)
	return lower == "experience" || lower == "work experience" || lower == "professional experience"
}

// isBulletLine returns true when the trimmed line starts with a recognized bullet marker.
func isBulletLine(trimmed string) bool {
	for _, m := range bulletMarkers {
		if strings.HasPrefix(trimmed, m) {
			return true
		}
	}
	return false
}

// bulletMarkerOf returns the leading bullet marker of a trimmed line, or "".
func bulletMarkerOf(trimmed string) string {
	for _, m := range bulletMarkers {
		if strings.HasPrefix(trimmed, m) {
			return m
		}
	}
	return ""
}

// bulletContainsAnyKeyword returns true when the bullet text mentions at least one keyword
// (case-insensitive substring match).
func bulletContainsAnyKeyword(bullet string, keywords []string) bool {
	lower := strings.ToLower(bullet)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// BulletRewriteInput groups all inputs for the bullet rewrite step.
// Struct required because the function takes ≥5 parameters.
type BulletRewriteInput struct {
	Ctx context.Context
	LLM port.LLMClient
	// Log is the injected structured logger used for per-bullet warnings.
	// Must not be nil; callers should pass slog.Default() in tests.
	Log                 *slog.Logger
	ResumeText          string
	JDKeywords          []string
	AccomplishmentsText string
	Defaults            *config.AppDefaults
	// MaxRewrites overrides Defaults.Tailor.MaxTier2BulletRewrites when > 0.
	// This avoids mutating the shared defaults pointer.
	MaxRewrites int
}

// rewriteBullets rewrites Experience bullets that are relevant to the JD keywords,
// grounded in the candidate's accomplishments. Returns the modified resume text,
// the list of changes, and any fatal error. LLM errors on individual bullets are
// logged and skipped — they do not abort the entire rewrite.
func rewriteBullets(input *BulletRewriteInput) (string, []model.BulletChange, error) {
	bullets := extractExperienceBullets(input.ResumeText)
	if len(bullets) == 0 {
		return input.ResumeText, nil, nil
	}

	maxRewrites := input.MaxRewrites
	if maxRewrites <= 0 {
		maxRewrites = input.Defaults.Tailor.MaxTier2BulletRewrites
	}
	modified := input.ResumeText
	changes := make([]model.BulletChange, 0, maxRewrites)
	rewroteCount := 0

	for _, originalLine := range bullets {
		if maxRewrites > 0 && rewroteCount >= maxRewrites {
			break
		}

		trimmed := strings.TrimSpace(originalLine)
		if !bulletContainsAnyKeyword(trimmed, input.JDKeywords) {
			continue
		}

		marker := bulletMarkerOf(trimmed)
		content := strings.TrimSpace(trimmed[len(marker):])

		prompt := fmt.Sprintf(
			"Rewrite the following resume bullet to better highlight impact and relevance to these skills: %s.\n"+
				"Use specific metrics and language from the candidate's accomplishments below.\n"+
				"Return ONLY the rewritten bullet text — no marker, no explanation.\n\n"+
				"Original bullet:\n%s\n\n"+
				"Candidate accomplishments:\n%s",
			strings.Join(input.JDKeywords, ", "),
			content,
			input.AccomplishmentsText,
		)

		messages := []model.ChatMessage{
			{Role: "system", Content: "You are an expert resume writer. Rewrite resume bullets to be results-driven and keyword-rich."},
			{Role: "user", Content: prompt},
		}
		opts := model.ChatOptions{
			Temperature: input.Defaults.LLM.BulletRewriteTemp,
			MaxTokens:   input.Defaults.LLM.BulletRewriteMaxTokens,
		}

		resp, err := input.LLM.ChatComplete(input.Ctx, messages, opts)
		if err != nil {
			input.Log.WarnContext(input.Ctx, "bullet rewrite LLM call failed — skipping bullet",
				"bullet", content, "error", err)
			continue
		}

		rewritten := strings.TrimSpace(resp)
		// Strip LLM's leading marker if it added one (e.g. "- rewritten content").
		for _, m := range bulletMarkers {
			if strings.HasPrefix(rewritten, m) {
				rewritten = strings.TrimSpace(rewritten[len(m):])
				break
			}
		}

		// Reconstruct line with original marker and indentation.
		leadingWhitespace := originalLine[:len(originalLine)-len(strings.TrimLeft(originalLine, " \t"))]
		rewrittenLine := leadingWhitespace + marker + " " + rewritten

		changes = append(changes, model.BulletChange{
			Original:  trimmed,
			Rewritten: strings.TrimSpace(rewrittenLine),
		})
		modified = strings.Replace(modified, originalLine, rewrittenLine, 1)
		rewroteCount++
	}

	return modified, changes, nil
}
