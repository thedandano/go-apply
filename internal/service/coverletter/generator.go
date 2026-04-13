package coverletter

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.CoverLetterGenerator = (*Generator)(nil)

// sentenceEnd matches terminal punctuation followed by whitespace or end-of-string.
// More accurate than counting every '.', '!', '?' — avoids false positives from
// decimal numbers ("3.5 years") and mid-sentence punctuation clusters.
var sentenceEnd = regexp.MustCompile(`[.!?]+(\s|$)`)

// Generator produces cover letters by calling an LLM with structured job and candidate context.
type Generator struct {
	llm      port.LLMClient
	defaults *config.AppDefaults
}

// New constructs a Generator with the provided LLM client and application defaults.
func New(llm port.LLMClient, defaults *config.AppDefaults) *Generator {
	return &Generator{
		llm:      llm,
		defaults: defaults,
	}
}

// Generate selects the highest-scoring resume, builds a prompt, calls the LLM, and returns
// a CoverLetterResult with the generated text plus word and sentence counts.
func (g *Generator) Generate(ctx context.Context, input *port.CoverLetterInput) (model.CoverLetterResult, error) {
	best := bestScore(input.Scores)
	prompt := buildPrompt(input, &best, g.defaults)

	messages := []port.ChatMessage{
		{
			Role:    "system",
			Content: "You are an expert cover letter writer. Write concise, authentic cover letters tailored to the job and candidate.",
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}

	opts := port.ChatOptions{
		Temperature: g.defaults.LLM.CoverLetterTemp,
		MaxTokens:   g.defaults.LLM.CoverLetterMaxTokens,
	}

	text, err := g.llm.ChatComplete(ctx, messages, opts)
	if err != nil {
		return model.CoverLetterResult{}, fmt.Errorf("cover letter llm call: %w", err)
	}
	if strings.TrimSpace(text) == "" {
		return model.CoverLetterResult{}, fmt.Errorf("cover letter llm call: empty response")
	}

	return model.CoverLetterResult{
		Text:          text,
		Channel:       input.Channel,
		WordCount:     countWords(text),
		SentenceCount: countSentences(text),
	}, nil
}

// bestScore returns the ScoreResult with the highest Breakdown.Total() from scores.
// Returns zero-value ScoreResult if scores is empty.
func bestScore(scores map[string]model.ScoreResult) model.ScoreResult {
	var best model.ScoreResult
	first := true
	for label := range scores {
		sr := scores[label]
		if first || sr.Breakdown.Total() > best.Breakdown.Total() {
			best = sr
			first = false
		}
	}
	return best
}

// buildPrompt composes the LLM user message from focused sub-sections.
func buildPrompt(input *port.CoverLetterInput, best *model.ScoreResult, defaults *config.AppDefaults) string {
	return buildJobSection(input) +
		buildCandidateSection(&input.Profile) +
		buildMatchSection(best) +
		buildInstruction(defaults)
}

// buildJobSection formats the job details and full JD text (when available).
func buildJobSection(input *port.CoverLetterInput) string {
	var sb strings.Builder
	sb.WriteString("Job Details:\n")
	sb.WriteString(fmt.Sprintf("  Title:    %s\n", input.JD.Title))
	sb.WriteString(fmt.Sprintf("  Company:  %s\n", input.JD.Company))
	sb.WriteString(fmt.Sprintf("  Location: %s\n", input.JD.Location))
	sb.WriteString(fmt.Sprintf("  Channel:  %s\n", string(input.Channel)))
	if len(input.JD.Required) > 0 {
		sb.WriteString("\nRequired Skills:\n  " + strings.Join(input.JD.Required, ", ") + "\n")
	}
	if len(input.JD.Preferred) > 0 {
		sb.WriteString("\nPreferred Skills:\n  " + strings.Join(input.JD.Preferred, ", ") + "\n")
	}
	if input.JDRawText != "" {
		sb.WriteString("\nFull Job Description:\n" + input.JDRawText + "\n")
	}
	return sb.String()
}

// buildCandidateSection formats the candidate profile fields.
func buildCandidateSection(profile *model.UserProfile) string {
	var sb strings.Builder
	sb.WriteString("\nCandidate:\n")
	sb.WriteString(fmt.Sprintf("  Name:       %s\n", profile.Name))
	sb.WriteString(fmt.Sprintf("  Occupation: %s\n", profile.Occupation))
	sb.WriteString(fmt.Sprintf("  Location:   %s\n", profile.Location))
	sb.WriteString(fmt.Sprintf("  Experience: %.0f years\n", profile.YearsOfExperience))
	return sb.String()
}

// buildMatchSection formats the matched keywords and score breakdown from the best resume.
func buildMatchSection(best *model.ScoreResult) string {
	var sb strings.Builder
	if len(best.Keywords.ReqMatched) > 0 {
		sb.WriteString("\nMatched Required Keywords:\n  " + strings.Join(best.Keywords.ReqMatched, ", ") + "\n")
	}
	if len(best.Keywords.PrefMatched) > 0 {
		sb.WriteString("\nMatched Preferred Keywords:\n  " + strings.Join(best.Keywords.PrefMatched, ", ") + "\n")
	}
	sb.WriteString("\nScore Breakdown:\n")
	sb.WriteString(fmt.Sprintf("  Keyword Match:   %.1f\n", best.Breakdown.KeywordMatch))
	sb.WriteString(fmt.Sprintf("  Experience Fit:  %.1f\n", best.Breakdown.ExperienceFit))
	sb.WriteString(fmt.Sprintf("  Impact Evidence: %.1f\n", best.Breakdown.ImpactEvidence))
	sb.WriteString(fmt.Sprintf("  ATS Format:      %.1f\n", best.Breakdown.ATSFormat))
	sb.WriteString(fmt.Sprintf("  Readability:     %.1f\n", best.Breakdown.Readability))
	sb.WriteString(fmt.Sprintf("  Total:           %.1f\n", best.Breakdown.Total()))
	return sb.String()
}

// buildInstruction returns the final cover letter writing directive.
func buildInstruction(defaults *config.AppDefaults) string {
	return fmt.Sprintf(
		"\nWrite a cover letter: target %d words (max %d), approximately %d sentences. "+
			"Sound human -- avoid corporate jargon and cliches. "+
			"Reference specific details from the job description above. "+
			"Do not use em-dashes; use a comma or rewrite the sentence instead.",
		defaults.CoverLetter.TargetWords,
		defaults.CoverLetter.MaxWords,
		defaults.CoverLetter.SentenceCount,
	)
}

// countWords returns the number of whitespace-delimited tokens in text.
func countWords(text string) int {
	return len(strings.Fields(text))
}

// countSentences counts sentences by matching terminal punctuation followed by
// whitespace or end-of-string. More accurate than counting raw punctuation characters.
func countSentences(text string) int {
	return len(sentenceEnd.FindAllString(strings.TrimSpace(text), -1))
}
