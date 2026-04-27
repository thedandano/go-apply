package profilecompiler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compiler implements port.ProfileCompiler using LLM skill tagging.
type Compiler struct {
	llm    port.LLMClient
	logger *slog.Logger
}

// New returns a Compiler. logger may be nil (uses slog.Default()).
func New(llm port.LLMClient, logger *slog.Logger) port.ProfileCompiler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Compiler{llm: llm, logger: logger}
}

var _ port.ProfileCompiler = (*Compiler)(nil)

// Compile tags each story with matching skills using the LLM and computes orphaned skills.
func (c *Compiler) Compile(ctx context.Context, input model.CompileInput) (model.CompiledProfile, error) {
	start := time.Now()
	skillSet := parseSkills(input.SkillsText)
	c.logger.InfoContext(ctx, "profilecompiler: compile start",
		slog.Int("story_count", len(input.Stories)),
		slog.Int("skill_count", len(skillSet)),
	)

	covered := map[string]bool{} // skills covered by at least one successful story
	stories := make([]model.Story, 0, len(input.Stories))
	partial := false

	for i, raw := range input.Stories {
		id := fmt.Sprintf("story-%03d", i+1)
		skills, taggingErr := c.tagStory(ctx, raw.Text, input.SkillsText, skillSet)
		story := model.Story{
			ID:           id,
			SourceFile:   raw.SourceFile,
			Text:         raw.Text,
			Skills:       skills,
			Format:       "SBI",
			TaggingError: taggingErr,
		}
		if taggingErr != "" {
			partial = true
			c.logger.WarnContext(ctx, "profilecompiler: tagging error",
				slog.String("story_id", id),
				slog.String("source_file", raw.SourceFile),
				slog.String("error", taggingErr),
			)
		} else {
			for _, sk := range skills {
				covered[sk] = true
			}
			c.logger.DebugContext(ctx, "profilecompiler: story tagged",
				slog.String("story_id", id),
				slog.Any("skills", skills),
			)
		}
		stories = append(stories, story)
	}

	orphaned := computeOrphans(skillSet, covered, input.PriorProfile)

	profile := model.CompiledProfile{
		SchemaVersion:         "1",
		CompiledAt:            time.Now().UTC(),
		Stories:               stories,
		OrphanedSkills:        orphaned,
		PartialTaggingFailure: partial,
	}

	c.logger.InfoContext(ctx, "profilecompiler: compile done",
		slog.Int("stories_tagged", len(stories)),
		slog.Int("orphan_count", len(orphaned)),
		slog.Bool("partial_tagging_failure", partial),
		slog.Duration("duration", time.Since(start)),
	)
	return profile, nil
}

// tagStory calls the LLM with a structured prompt and returns filtered skill labels.
// On any failure it returns (nil, errorDescription).
func (c *Compiler) tagStory(ctx context.Context, storyText, skillsText string, skillSet map[string]bool) ([]string, string) {
	prompt := fmt.Sprintf("skills:\n%s\n\nstory:\n%s", skillsText, storyText)
	msgs := []model.ChatMessage{
		{Role: "system", Content: "You are a skill tagger. Given a list of skills and a story, return a JSON array of skill labels from the list that are evidenced by the story. Return only labels that appear verbatim in the provided skills list. Return an empty JSON array if none match. No explanation."},
		{Role: "user", Content: prompt},
	}
	opts := model.ChatOptions{Temperature: 0, MaxTokens: 512}
	resp, err := c.llm.ChatComplete(ctx, msgs, opts)
	if err != nil {
		return nil, fmt.Sprintf("llm error: %v", err)
	}

	var labels []string
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(resp)), &labels); jsonErr != nil {
		return nil, fmt.Sprintf("parse error: %v (response: %.100s)", jsonErr, resp)
	}

	// Filter: keep only labels that appear in the skills source.
	var filtered []string
	for _, label := range labels {
		if skillSet[label] {
			filtered = append(filtered, label)
		}
	}
	if filtered == nil {
		filtered = []string{}
	}
	return filtered, ""
}

// parseSkills splits the raw skills text into a set of non-empty labels.
func parseSkills(text string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		label := strings.TrimSpace(line)
		if label != "" && !strings.HasPrefix(label, "#") {
			set[label] = true
		}
	}
	return set
}

// computeOrphans finds skills with no successfully-tagged story.
// deferred flags are carried forward from priorProfile.
func computeOrphans(skillSet, covered map[string]bool, prior *model.CompiledProfile) []model.OrphanedSkill {
	priorDeferred := map[string]bool{}
	if prior != nil {
		for _, o := range prior.OrphanedSkills {
			if o.Deferred {
				priorDeferred[o.Skill] = true
			}
		}
	}

	var orphans []model.OrphanedSkill
	for skill := range skillSet {
		if !covered[skill] {
			orphans = append(orphans, model.OrphanedSkill{
				Skill:    skill,
				Deferred: priorDeferred[skill],
			})
		}
	}
	return orphans
}
