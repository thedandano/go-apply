package coverletter

import (
	"context"
	"fmt"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.CoverLetterGenerator = (*Generator)(nil)

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

	prompt := buildPrompt(input, &best)

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

// buildPrompt constructs the user message for the LLM containing job, candidate, and match context.
func buildPrompt(input *port.CoverLetterInput, best *model.ScoreResult) string {
	var sb strings.Builder

	sb.WriteString("Job Details:\n")
	sb.WriteString(fmt.Sprintf("  Title:    %s\n", input.JD.Title))
	sb.WriteString(fmt.Sprintf("  Company:  %s\n", input.JD.Company))
	sb.WriteString(fmt.Sprintf("  Location: %s\n", input.JD.Location))
	sb.WriteString(fmt.Sprintf("  Channel:  %s\n", string(input.Channel)))

	sb.WriteString("\nCandidate:\n")
	sb.WriteString(fmt.Sprintf("  Name:       %s\n", input.Profile.Name))
	sb.WriteString(fmt.Sprintf("  Occupation: %s\n", input.Profile.Occupation))
	sb.WriteString(fmt.Sprintf("  Location:   %s\n", input.Profile.Location))
	sb.WriteString(fmt.Sprintf("  Experience: %.0f years\n", input.Profile.YearsOfExperience))

	if len(best.Keywords.ReqMatched) > 0 {
		sb.WriteString("\nMatched Required Keywords:\n")
		sb.WriteString("  " + strings.Join(best.Keywords.ReqMatched, ", ") + "\n")
	}

	if len(best.Keywords.PrefMatched) > 0 {
		sb.WriteString("\nMatched Preferred Keywords:\n")
		sb.WriteString("  " + strings.Join(best.Keywords.PrefMatched, ", ") + "\n")
	}

	sb.WriteString("\nScore Breakdown:\n")
	sb.WriteString(fmt.Sprintf("  Keyword Match:   %.1f\n", best.Breakdown.KeywordMatch))
	sb.WriteString(fmt.Sprintf("  Experience Fit:  %.1f\n", best.Breakdown.ExperienceFit))
	sb.WriteString(fmt.Sprintf("  Impact Evidence: %.1f\n", best.Breakdown.ImpactEvidence))
	sb.WriteString(fmt.Sprintf("  ATS Format:      %.1f\n", best.Breakdown.ATSFormat))
	sb.WriteString(fmt.Sprintf("  Readability:     %.1f\n", best.Breakdown.Readability))
	sb.WriteString(fmt.Sprintf("  Total:           %.1f\n", best.Breakdown.Total()))

	sb.WriteString("\nWrite a concise, authentic cover letter for this candidate and role.")

	return sb.String()
}

// countWords returns the number of whitespace-delimited tokens in text.
func countWords(text string) int {
	return len(strings.Fields(text))
}

// countSentences counts terminal punctuation characters (., !, ?) as a sentence-count heuristic.
func countSentences(text string) int {
	count := 0
	for _, ch := range text {
		if ch == '.' || ch == '!' || ch == '?' {
			count++
		}
	}
	return count
}
