package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/profilecompiler"
)

// HandleCompileProfile is the MCP-wired handler for the compile_profile tool.
// It parses JSON-string arguments (skills, remove_skills, stories) and delegates
// to HandleCompileProfileWith which loads the prior profile from disk.
func HandleCompileProfile(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	args := req.GetArguments()

	var skills []string
	if s, ok := args["skills"].(string); ok && s != "" {
		if err := json.Unmarshal([]byte(s), &skills); err != nil {
			return errorResult("skills: invalid JSON: " + err.Error())
		}
	}

	var removeSkills []string
	if s, ok := args["remove_skills"].(string); ok && s != "" {
		if err := json.Unmarshal([]byte(s), &removeSkills); err != nil {
			return errorResult("remove_skills: invalid JSON: " + err.Error())
		}
	}

	var assembleStories []model.AssembleStory
	if s, ok := args["stories"].(string); ok && s != "" {
		if err := json.Unmarshal([]byte(s), &assembleStories); err != nil {
			return errorResult("stories: invalid JSON: " + err.Error())
		}
	}

	return HandleCompileProfileWith(ctx, config.DataDir(), model.AssembleInput{
		Skills:       skills,
		RemoveSkills: removeSkills,
		Stories:      assembleStories,
	})
}

// HandleCompileProfileWith is the testable core — loads the prior profile from dataDir,
// runs the assembler with the provided input, saves the result, and returns a rich diff.
// Tests pass AssembleInput directly to bypass the JSON parse layer in HandleCompileProfile.
//
//nolint:gocritic // hugeParam: AssembleInput passed by value; pointer would complicate caller sites
func HandleCompileProfileWith(ctx context.Context, dataDir string, input model.AssembleInput) *mcp.CallToolResult {
	repo := fs.NewCompiledProfileRepository()
	compiler := profilecompiler.New(slog.Default())

	// Load prior for ID resolution and diff computation; missing profile is not an error.
	prior, err := repo.Load(dataDir)
	if err == nil {
		input.PriorProfile = &prior
	} else if !errors.Is(err, model.ErrProfileMissing) {
		return errorResult(fmt.Sprintf("load compiled profile: %v", err))
	}

	slog.DebugContext(ctx, "compile_profile: called",
		slog.Int("skills", len(input.Skills)),
		slog.Int("stories", len(input.Stories)),
		slog.Bool("has_prior", input.PriorProfile != nil),
	)

	profile, compileErr := compiler.Compile(ctx, input)
	if compileErr != nil {
		return errorResult(compileErr.Error())
	}

	if saveErr := repo.Save(dataDir, profile); saveErr != nil {
		slog.ErrorContext(ctx, "compile_profile: save failed", slog.String("error", saveErr.Error()))
		return errorResult(fmt.Sprintf("save compiled profile: %v", saveErr))
	}

	return buildRichDiffResult(input, profile)
}

// buildRichDiffResult assembles the compile_profile response with diff fields computed
// relative to the prior profile (stored in input.PriorProfile).
//
//nolint:gocritic // hugeParam: AssembleInput and CompiledProfile passed by value for immutability
func buildRichDiffResult(input model.AssembleInput, profile model.CompiledProfile) *mcp.CallToolResult {
	prior := input.PriorProfile

	var skillsAdded, skillsRemoved, coverageGained []string

	if prior != nil {
		priorSkillSet := stringSet(prior.Skills)
		newSkillSet := stringSet(profile.Skills)

		priorOrphanSet := map[string]bool{}
		for _, o := range prior.OrphanedSkills {
			priorOrphanSet[o.Skill] = true
		}
		newOrphanSet := map[string]bool{}
		for _, o := range profile.OrphanedSkills {
			newOrphanSet[o.Skill] = true
		}

		for _, sk := range profile.Skills {
			if !priorSkillSet[sk] {
				skillsAdded = append(skillsAdded, sk)
			}
		}
		for _, sk := range prior.Skills {
			if !newSkillSet[sk] {
				skillsRemoved = append(skillsRemoved, sk)
			}
		}
		// coverage_gained: skills that were orphaned in prior and are now covered.
		for skill := range priorOrphanSet {
			if !newOrphanSet[skill] {
				coverageGained = append(coverageGained, skill)
			}
		}
		sort.Strings(skillsAdded)
		sort.Strings(skillsRemoved)
		sort.Strings(coverageGained)
	}

	storiesAdded, storiesUpdated := 0, 0
	for _, s := range input.Stories {
		if s.ID != "" {
			storiesUpdated++
		} else {
			storiesAdded++
		}
	}

	type orphanOut struct {
		Skill    string `json:"skill"`
		Deferred bool   `json:"deferred"`
	}
	orphans := make([]orphanOut, len(profile.OrphanedSkills))
	for i, o := range profile.OrphanedSkills {
		orphans[i] = orphanOut{Skill: o.Skill, Deferred: o.Deferred}
	}

	if skillsAdded == nil {
		skillsAdded = []string{}
	}
	if skillsRemoved == nil {
		skillsRemoved = []string{}
	}
	if coverageGained == nil {
		coverageGained = []string{}
	}

	out := map[string]interface{}{
		"status":          "compiled",
		"compiled_at":     profile.CompiledAt,
		"orphaned_skills": orphans,
		"coverage_gained": coverageGained,
		"skills_added":    skillsAdded,
		"skills_removed":  skillsRemoved,
		"stories_added":   storiesAdded,
		"stories_updated": storiesUpdated,
	}

	data, err := json.Marshal(out)
	if err != nil {
		return errorResult("marshal response: " + err.Error())
	}
	return mcp.NewToolResultText(string(data))
}

// stringSet converts a string slice to a presence map.
func stringSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
