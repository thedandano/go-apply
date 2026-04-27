package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/profilecompiler"
)

// HandleCompileProfile is the MCP-wired handler for the compile_profile tool.
func HandleCompileProfile(ctx context.Context, _ *mcp.CallToolRequest) *mcp.CallToolResult {
	cfg, _, err := loadDeps()
	if err != nil {
		return errorResult("load deps: " + err.Error())
	}
	llmClient, err := newLLMClient(cfg)
	if err != nil {
		return errorResult("llm client: " + err.Error())
	}
	return HandleCompileProfileWith(ctx, config.DataDir(), llmClient)
}

// HandleCompileProfileWith is the testable core — accepts explicit dataDir and LLM client.
func HandleCompileProfileWith(ctx context.Context, dataDir string, llmClient port.LLMClient) *mcp.CallToolResult {
	repo := fs.NewCompiledProfileRepository()
	compiler := profilecompiler.New(llmClient, slog.Default())

	// 1. Load prior profile.
	prior, err := repo.Load(dataDir)
	var priorPtr *model.CompiledProfile
	if err == nil {
		priorPtr = &prior
		// 2. Staleness check — only when profile exists.
		stale, _, staleErr := repo.IsStale(dataDir)
		if staleErr == nil && !stale {
			// Profile is current — return existing data without recompiling.
			return compileProfileResult("already_up_to_date", &prior)
		}
	} else if !errors.Is(err, model.ErrProfileMissing) {
		return errorResult("load compiled profile: " + err.Error())
	}
	// ErrProfileMissing → priorPtr remains nil → always compile.

	// 3. Read source files.
	skillsText, rawStories, readErr := readSourceFiles(dataDir)
	if readErr != nil {
		return errorResult("read source files: " + readErr.Error())
	}

	// 4. Compile.
	input := model.CompileInput{
		SkillsText:   skillsText,
		Stories:      rawStories,
		PriorProfile: priorPtr,
	}
	compiled, compileErr := compiler.Compile(ctx, input)
	if compileErr != nil {
		return errorResult("compile: " + compileErr.Error())
	}

	// 5. Save atomically.
	if saveErr := repo.Save(dataDir, compiled); saveErr != nil {
		slog.WarnContext(ctx, "compile_profile: save failed", slog.String("error", saveErr.Error()))
		return errorResult("save compiled profile: " + saveErr.Error())
	}

	return compileProfileResult("compiled", &compiled)
}

// readSourceFiles reads skills.md and all accomplishments-N.md files from dataDir.
func readSourceFiles(dataDir string) (string, []model.RawStory, error) {
	skillsPath := filepath.Join(dataDir, "skills.md")
	skillsData, err := os.ReadFile(skillsPath) // #nosec G304
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	skillsText := string(skillsData)

	matches, _ := filepath.Glob(filepath.Join(dataDir, "accomplishments-*.md"))
	stories := make([]model.RawStory, 0, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path) // #nosec G304
		if err != nil {
			continue
		}
		stories = append(stories, model.RawStory{
			SourceFile: filepath.Base(path),
			Text:       string(data),
		})
	}
	return skillsText, stories, nil
}

// compileProfileResult marshals the profile into the compile_profile contract response.
func compileProfileResult(status string, p *model.CompiledProfile) *mcp.CallToolResult {
	type storyOut struct {
		ID           string          `json:"id"`
		SourceFile   string          `json:"source_file"`
		Text         string          `json:"text"`
		Skills       []string        `json:"skills"`
		Format       string          `json:"format"`
		Type         model.StoryType `json:"type"`
		JobTitle     string          `json:"job_title"`
		TaggingError string          `json:"tagging_error"`
	}
	type orphanOut struct {
		Skill    string `json:"skill"`
		Deferred bool   `json:"deferred"`
	}

	stories := make([]storyOut, len(p.Stories))
	for i := range p.Stories {
		s := &p.Stories[i]
		sk := s.Skills
		if sk == nil {
			sk = []string{}
		}
		stories[i] = storyOut{
			ID:           s.ID,
			SourceFile:   s.SourceFile,
			Text:         s.Text,
			Skills:       sk,
			Format:       s.Format,
			Type:         s.Type,
			JobTitle:     s.JobTitle,
			TaggingError: s.TaggingError,
		}
	}
	orphans := make([]orphanOut, len(p.OrphanedSkills))
	for i, o := range p.OrphanedSkills {
		orphans[i] = orphanOut{Skill: o.Skill, Deferred: o.Deferred}
	}

	out := map[string]interface{}{
		"schema_version":          "1",
		"status":                  status,
		"compiled_at":             p.CompiledAt,
		"partial_tagging_failure": p.PartialTaggingFailure,
		"stories":                 stories,
		"orphaned_skills":         orphans,
	}
	data, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(data))
}
