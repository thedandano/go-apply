// Package storycreator writes new accomplishment stories to accomplishments.json and maintains career.json.
package storycreator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

const accomplishmentsFile = "accomplishments.json"

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

// Create validates input, optionally appends career.json, and appends the story to accomplishments.json.
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

	// 3. Load accomplishments.json before any mutations so a corrupt file cannot leave career.json mutated.
	acc, err := s.loadAccomplishments()
	if err != nil {
		return model.StoryOutput{}, err
	}

	// 4. Validate / register job title via career.json.
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

	// 5. Compute new story ID and append to AccomplishmentsJSON.
	id, err := nextStoryID(acc)
	if err != nil {
		return model.StoryOutput{}, err
	}
	content := formatStory(input)
	acc.CreatedStories = append(acc.CreatedStories, model.CreatedStory{
		ID:       id,
		Skill:    input.PrimarySkill,
		Type:     input.StoryType,
		JobTitle: input.JobTitle,
		Text:     content,
	})

	// 6. Write accomplishments.json atomically.
	if err := s.saveAccomplishments(acc); err != nil {
		return model.StoryOutput{}, err
	}

	s.log.Info("storycreator: story written",
		slog.String("story_id", id),
		slog.String("primary_skill", input.PrimarySkill),
		slog.String("job_title", input.JobTitle),
	)
	return model.StoryOutput{StoryID: id}, nil
}

// loadAccomplishments reads accomplishments.json from dataDir.
// Missing file is treated as an empty AccomplishmentsJSON with schema version "1".
// Malformed JSON is a hard error.
func (s *Service) loadAccomplishments() (model.AccomplishmentsJSON, error) {
	path := filepath.Join(s.dataDir, accomplishmentsFile)
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return model.AccomplishmentsJSON{SchemaVersion: "1"}, nil
		}
		return model.AccomplishmentsJSON{}, fmt.Errorf("read accomplishments: %w", err)
	}
	var acc model.AccomplishmentsJSON
	if err := json.Unmarshal(data, &acc); err != nil {
		return model.AccomplishmentsJSON{}, fmt.Errorf("parse accomplishments: %w", err)
	}
	if acc.SchemaVersion != model.AccomplishmentsSchemaV1 {
		return model.AccomplishmentsJSON{}, fmt.Errorf("accomplishments.json schema_version %q is not supported — run go-apply onboard --reset", acc.SchemaVersion)
	}
	return acc, nil
}

// saveAccomplishments writes acc to accomplishments.json atomically using a temp file + rename.
func (s *Service) saveAccomplishments(acc model.AccomplishmentsJSON) error {
	data, err := json.Marshal(acc)
	if err != nil {
		return fmt.Errorf("marshal accomplishments: %w", err)
	}
	path := filepath.Join(s.dataDir, accomplishmentsFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G306
		return fmt.Errorf("write accomplishments tmp: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }()
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename accomplishments: %w", err)
	}
	return nil
}

// nextStoryID returns the next integer ID string for a new story entry.
// It scans acc.CreatedStories and finds the max numeric ID (default -1 when empty),
// then returns strconv.Itoa(max+1). Non-numeric IDs are a hard error.
func nextStoryID(acc model.AccomplishmentsJSON) (string, error) {
	maxID := -1
	for _, s := range acc.CreatedStories {
		n, err := strconv.Atoi(s.ID)
		if err != nil {
			return "", fmt.Errorf("corrupt story id %q in accomplishments.json: %w", s.ID, err)
		}
		if n > maxID {
			maxID = n
		}
	}
	return strconv.Itoa(maxID + 1), nil
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

// formatStory renders the SBI text for a created story entry.
func formatStory(input model.StoryInput) string { //nolint:gocritic // hugeParam: internal helper — pointer would add noise
	return fmt.Sprintf("## %s — %s @ %s\n**Situation:** %s\n**Behavior:** %s\n**Impact:** %s\n",
		input.PrimarySkill, input.StoryType, input.JobTitle,
		input.Situation, input.Behavior, input.Impact,
	)
}
