package onboarding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// OnboardFile carries one resume's data for onboarding.
type OnboardFile struct {
	Label     string // e.g. "backend"
	PlainText string // pre-extracted by DocumentLoader
	OrigPath  string // source path — for copying to inputs/
	Format    string // ".docx", ".pdf", ".md", etc.
}

// OnboardInput is the full input for one onboarding run.
// All file contents are pre-extracted strings — no file I/O in the service.
type OnboardInput struct {
	Resumes             map[string]OnboardFile // label → file
	SkillsText          string                 // empty = skip
	AccomplishmentsText string                 // empty = skip
	Profile             *model.UserProfile     // nil = skip config update
}

// OnboardResult reports what was stored.
type OnboardResult struct {
	ResumesStored         []string
	SkillsStored          bool
	AccomplishmentsStored bool
	EmbeddingsIndexed     int
	Warnings              []string
}

// Service stores user files, indexes embeddings into sqlite-vec.
type Service struct {
	profile  port.ProfileRepository
	embedder port.EmbeddingClient
	dataDir  string
}

// New returns a new onboarding Service.
func New(profile port.ProfileRepository, embedder port.EmbeddingClient, dataDir string) *Service {
	return &Service{
		profile:  profile,
		embedder: embedder,
		dataDir:  dataDir,
	}
}

// Run executes the full onboarding sequence:
// For each resume: write plain text to inputs/<label>.txt, embed, upsert into sqlite-vec.
// For skills/accomplishments: write to dataDir, embed, upsert.
// Returns OnboardResult; individual doc failures are collected as Warnings, not fatal errors.
func (s *Service) Run(ctx context.Context, input OnboardInput) (OnboardResult, error) {
	var result OnboardResult

	inputsDir := filepath.Join(s.dataDir, "inputs")
	if err := os.MkdirAll(inputsDir, 0o700); err != nil {
		return result, fmt.Errorf("create inputs dir %s: %w", inputsDir, err)
	}

	// Process resumes
	for label, file := range input.Resumes {
		destPath := filepath.Join(inputsDir, label+".txt")
		if err := os.WriteFile(destPath, []byte(file.PlainText), 0o600); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("write resume %s: %v", label, err))
			continue
		}

		sourceDoc := "resume:" + label
		if err := s.embedAndUpsert(ctx, sourceDoc, file.PlainText); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("embed/upsert resume %s: %v", label, err))
			continue
		}

		result.ResumesStored = append(result.ResumesStored, label)
		result.EmbeddingsIndexed++
	}

	// Process skills
	if input.SkillsText != "" {
		skillsPath := filepath.Join(s.dataDir, "skills.md")
		if err := os.WriteFile(skillsPath, []byte(input.SkillsText), 0o600); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("write skills: %v", err))
		} else if err := s.embedAndUpsert(ctx, "skills", input.SkillsText); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("embed/upsert skills: %v", err))
		} else {
			result.SkillsStored = true
			result.EmbeddingsIndexed++
		}
	}

	// Process accomplishments
	if input.AccomplishmentsText != "" {
		accomplishmentsPath := filepath.Join(s.dataDir, "accomplishments.md")
		if err := os.WriteFile(accomplishmentsPath, []byte(input.AccomplishmentsText), 0o600); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("write accomplishments: %v", err))
		} else if err := s.embedAndUpsert(ctx, "accomplishments", input.AccomplishmentsText); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("embed/upsert accomplishments: %v", err))
		} else {
			result.AccomplishmentsStored = true
			result.EmbeddingsIndexed++
		}
	}

	return result, nil
}

// embedAndUpsert embeds text and upserts it into the profile repository.
func (s *Service) embedAndUpsert(ctx context.Context, sourceDoc, text string) error {
	vec, err := s.embedder.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if err := s.profile.UpsertDocument(ctx, sourceDoc, text, vec); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	return nil
}
