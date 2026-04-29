package profilecompiler

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compiler implements port.ProfileCompiler as a pure host-driven assembler.
// No LLM calls — skill tagging is performed by the host before Compile is invoked.
type Compiler struct {
	logger *slog.Logger
}

// New returns a Compiler. logger may be nil (uses slog.Default()).
// Return type is *Compiler until task 3.5 updates port.ProfileCompiler to AssembleInput.
func New(logger *slog.Logger) *Compiler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Compiler{logger: logger}
}

var _ port.ProfileCompiler = (*Compiler)(nil)

// Compile assembles a CompiledProfile from host-tagged input.
// It resolves existing story IDs from the prior profile and assigns new IDs to new stories.
// Returns an error if any story ID cannot be resolved in the prior profile.
//
//nolint:gocritic // hugeParam: AssembleInput by value; pointer would break port.ProfileCompiler interface
func (c *Compiler) Compile(ctx context.Context, input model.AssembleInput) (model.CompiledProfile, error) {
	start := time.Now()

	// Compute effective skills: union(prior, input.Skills) - input.RemoveSkills.
	effectiveSkills := computeEffectiveSkills(input)

	c.logger.InfoContext(ctx, "profilecompiler: compile start",
		slog.Int("story_count", len(input.Stories)),
		slog.Int("skill_count", len(effectiveSkills)),
	)

	// Determine next ID index: 1 + count of stories in prior profile.
	nextIndex := 1
	if input.PriorProfile != nil {
		nextIndex = len(input.PriorProfile.Stories) + 1
	}

	// Assemble stories.
	assembled := make([]model.Story, 0, len(input.Stories))
	for _, s := range input.Stories {
		var story model.Story
		if s.ID != "" {
			// Resolve from prior profile.
			if input.PriorProfile == nil {
				return model.CompiledProfile{}, fmt.Errorf("unknown story ID: %q (no prior profile)", s.ID)
			}
			prior, found := findStory(input.PriorProfile.Stories, s.ID)
			if !found {
				return model.CompiledProfile{}, fmt.Errorf("unknown story ID: %q", s.ID)
			}
			story = model.Story{
				ID:         prior.ID,
				SourceFile: s.Source,
				Text:       prior.Text,
				Skills:     s.Tags,
				Format:     "SBI",
			}
		} else {
			// New story: assign next ID.
			story = model.Story{
				ID:         fmt.Sprintf("story-%03d", nextIndex),
				SourceFile: s.Source,
				Text:       s.Accomplishment,
				Skills:     s.Tags,
				Format:     "SBI",
			}
			nextIndex++
		}
		assembled = append(assembled, story)

		c.logger.DebugContext(ctx, "profilecompiler: story assembled",
			slog.String("story_id", story.ID),
			slog.Any("skills", story.Skills),
		)
	}

	// Build covered set from all assembled story tags.
	covered := map[string]bool{}
	for _, s := range assembled {
		for _, sk := range s.Skills {
			covered[sk] = true
		}
	}

	orphans := computeOrphans(effectiveSkills, covered, input.PriorProfile)

	profile := model.CompiledProfile{
		SchemaVersion:  "1",
		CompiledAt:     time.Now().UTC(),
		Skills:         effectiveSkills,
		Stories:        assembled,
		OrphanedSkills: orphans,
	}

	c.logger.InfoContext(ctx, "profilecompiler: compile done",
		slog.Int("stories_assembled", len(assembled)),
		slog.Int("orphan_count", len(orphans)),
		slog.Duration("duration", time.Since(start)),
	)
	return profile, nil
}

// computeEffectiveSkills returns sort(set(prior.Skills) ∪ set(input.Skills) - set(input.RemoveSkills)).
//
//nolint:gocritic // hugeParam: internal helper; pointer would propagate to public Compile signature
func computeEffectiveSkills(input model.AssembleInput) []string {
	merged := map[string]bool{}

	if input.PriorProfile != nil {
		for _, sk := range input.PriorProfile.Skills {
			merged[sk] = true
		}
	}
	for _, sk := range input.Skills {
		merged[sk] = true
	}
	remove := toSet(input.RemoveSkills)
	for sk := range remove {
		delete(merged, sk)
	}

	result := make([]string, 0, len(merged))
	for sk := range merged {
		result = append(result, sk)
	}
	sort.Strings(result)
	return result
}

// computeOrphans returns skills in effectiveSkills with no story coverage.
// Deferred flags are carried forward from the prior profile.
func computeOrphans(effectiveSkills []string, covered map[string]bool, prior *model.CompiledProfile) []model.OrphanedSkill {
	priorDeferred := map[string]bool{}
	if prior != nil {
		for _, o := range prior.OrphanedSkills {
			if o.Deferred {
				priorDeferred[o.Skill] = true
			}
		}
	}

	var orphans []model.OrphanedSkill
	for _, skill := range effectiveSkills {
		if !covered[skill] {
			orphans = append(orphans, model.OrphanedSkill{
				Skill:    skill,
				Deferred: priorDeferred[skill],
			})
		}
	}
	return orphans
}

// findStory returns the story with the given ID from a slice.
func findStory(stories []model.Story, id string) (model.Story, bool) {
	for _, s := range stories {
		if s.ID == id {
			return s, true
		}
	}
	return model.Story{}, false
}

// toSet converts a string slice to a presence map.
func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
