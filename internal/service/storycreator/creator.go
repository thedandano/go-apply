// Package storycreator writes new accomplishment stories to disk and maintains career.json.
package storycreator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Service implements port.StoryCreatorService.
type Service struct {
	dataDir    string
	careerRepo port.CareerRepository
	log        *slog.Logger
}

// New returns a Service writing to dataDir.
// log may be nil (defaults to slog.Default).
func New(dataDir string, careerRepo port.CareerRepository, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{dataDir: dataDir, careerRepo: careerRepo, log: log}
}

var _ port.StoryCreatorService = (*Service)(nil)

// Create validates input, optionally appends career.json, and writes the story file.
func (s *Service) Create(_ context.Context, input model.StoryInput) (model.StoryOutput, error) { //nolint:gocritic // hugeParam: StoryInput is 136B — interface signature fixed
	// 1. Validate non-empty SBI fields.
	for field, val := range map[string]string{
		"situation": input.Situation,
		"behavior":  input.Behavior,
		"impact":    input.Impact,
	} {
		if strings.TrimSpace(val) == "" {
			return model.StoryOutput{}, fmt.Errorf("empty field: %s", field)
		}
	}

	// 2. Validate primary skill exists in skills.md.
	if err := s.validateSkill(input.PrimarySkill); err != nil {
		return model.StoryOutput{}, err
	}

	// 3. Validate / register job title via career.json.
	if input.IsNewJob {
		if err := s.careerRepo.AppendExperience(s.dataDir, model.ExperienceRef{
			Role:      input.JobTitle,
			StartDate: input.StartDate,
			EndDate:   input.EndDate,
		}); err != nil {
			return model.StoryOutput{}, fmt.Errorf("append career: %w", err)
		}
	} else {
		ok, err := s.careerRepo.HasExperience(s.dataDir, input.JobTitle)
		if err != nil {
			return model.StoryOutput{}, fmt.Errorf("check career: %w", err)
		}
		if !ok {
			return model.StoryOutput{}, fmt.Errorf("job title not found — set is_new_job=true: %q", input.JobTitle)
		}
	}

	// 4. Write story to next available accomplishments-N.md.
	path, base, err := s.nextAccomplishmentsPath()
	if err != nil {
		return model.StoryOutput{}, fmt.Errorf("next accomplishments path: %w", err)
	}
	content := formatStory(input)
	if writeErr := os.WriteFile(path, []byte(content), 0o600); writeErr != nil { // #nosec G306
		return model.StoryOutput{}, fmt.Errorf("write story file: %w", writeErr)
	}

	s.log.Info("storycreator: story written",
		slog.String("source_file", base),
		slog.String("primary_skill", input.PrimarySkill),
		slog.String("job_title", input.JobTitle),
	)
	return model.StoryOutput{SourceFile: base}, nil
}

// validateSkill checks that primarySkill appears verbatim as a non-comment line in skills.md.
func (s *Service) validateSkill(primarySkill string) error {
	data, err := os.ReadFile(filepath.Join(s.dataDir, "skills.md")) // #nosec G304
	if err != nil {
		return fmt.Errorf("skill not found in skills source (skills.md missing): %w", err)
	}
	needle := strings.TrimSpace(primarySkill)
	for _, line := range strings.Split(string(data), "\n") {
		label := strings.TrimSpace(line)
		if label == "" || strings.HasPrefix(label, "#") {
			continue
		}
		if label == needle {
			return nil
		}
	}
	return fmt.Errorf("skill not found in skills source: %q", primarySkill)
}

// nextAccomplishmentsPath returns the absolute path and basename of the next accomplishments file.
// Files are named accomplishments-N.md. The counter starts at 0 and advances past the highest
// existing N, including gaps.
func (s *Service) nextAccomplishmentsPath() (string, string, error) {
	matches, _ := filepath.Glob(filepath.Join(s.dataDir, "accomplishments-*.md"))
	maxN := -1
	for _, path := range matches {
		var n int
		if _, err := fmt.Sscanf(filepath.Base(path), "accomplishments-%d.md", &n); err == nil {
			if n > maxN {
				maxN = n
			}
		}
	}
	base := fmt.Sprintf("accomplishments-%d.md", maxN+1)
	return filepath.Join(s.dataDir, base), base, nil
}

// formatStory renders the SBI text for the accomplishments file.
func formatStory(input model.StoryInput) string { //nolint:gocritic // hugeParam: internal helper — pointer would add noise
	return fmt.Sprintf("## %s — %s @ %s\n**Situation:** %s\n**Behavior:** %s\n**Impact:** %s\n",
		input.PrimarySkill, input.StoryType, input.JobTitle,
		input.Situation, input.Behavior, input.Impact,
	)
}
