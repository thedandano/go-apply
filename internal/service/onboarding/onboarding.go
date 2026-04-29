// Package onboarding stores resume, skills, and accomplishments text as files
// on disk. It no longer embeds documents into a vector store.
package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Onboarder = (*Service)(nil)

// Service implements port.Onboarder.
// It writes text files to disk:
//   - Resumes → dataDir/inputs/<label>.txt
//   - Skills  → dataDir/skills.md
//   - Accomplishments → dataDir/accomplishments.json
type Service struct {
	dataDir string
	log     *slog.Logger
}

// New constructs a Service that writes profile documents to dataDir.
// dataDir should be config.DataDir().
func New(dataDir string, log *slog.Logger) *Service {
	return &Service{
		dataDir: dataDir,
		log:     log,
	}
}

// Run stores all provided resumes, skills, and accomplishments as files on disk.
// Failures for individual documents are collected as Warnings; Run always returns nil error.
func (s *Service) Run(ctx context.Context, input model.OnboardInput) (model.OnboardResult, error) {
	var result model.OnboardResult

	inputsDir := filepath.Join(s.dataDir, "inputs")
	if err := os.MkdirAll(inputsDir, config.DirPerm); err != nil {
		return result, fmt.Errorf("create inputs dir: %w", err)
	}

	for _, resume := range input.Resumes {
		if err := validateLabel(resume.Label); err != nil {
			s.log.DebugContext(ctx, "onboard: skipping resume — invalid label",
				slog.String("label", resume.Label),
			)
			result.Warnings = append(result.Warnings, model.RiskWarning{
				Severity: model.SeverityError,
				Message:  fmt.Sprintf("resume %q: %v", resume.Label, err),
			})
			continue
		}
		sourceDoc := "resume:" + resume.Label
		path := filepath.Join(inputsDir, resume.Label+".txt")
		s.log.DebugContext(ctx, "onboard: storing resume", "source", sourceDoc, "input_bytes", len(resume.Text))
		if warn := s.writeDocument(ctx, sourceDoc, resume.Text, path); warn != "" {
			result.Warnings = append(result.Warnings, model.RiskWarning{Severity: model.SeverityWarn, Message: warn})
			continue
		}
		s.log.DebugContext(ctx, "onboard: resume stored", "source", sourceDoc)
		result.Stored = append(result.Stored, sourceDoc)
		result.Summary.ResumesAdded++
	}

	if input.SkillsText != "" {
		path := filepath.Join(s.dataDir, "skills.md")
		s.log.DebugContext(ctx, "onboard: storing skills", "input_bytes", len(input.SkillsText))
		if warn := s.writeDocument(ctx, "ref:skills", input.SkillsText, path); warn != "" {
			result.Warnings = append(result.Warnings, model.RiskWarning{Severity: model.SeverityWarn, Message: warn})
		} else {
			s.log.DebugContext(ctx, "onboard: skills stored")
			result.Stored = append(result.Stored, "ref:skills")
		}
	} else {
		s.log.DebugContext(ctx, "onboard: skipping skills — empty input")
	}

	if input.AccomplishmentsText != "" {
		if err := s.writeAccomplishments(ctx, input.AccomplishmentsText); err != nil {
			return result, err
		}
		result.Stored = append(result.Stored, "accomplishments:onboard")
		result.Summary.AccomplishmentsCount = 1
		s.log.DebugContext(ctx, "onboard: accomplishments stored")
	} else {
		s.log.DebugContext(ctx, "onboard: skipping accomplishments — empty input")
	}

	result.Summary.SkillsCount = countSkillItems(input.SkillsText)
	result.Summary.TotalChunks = len(result.Stored)

	return result, nil
}

// writeAccomplishments loads existing accomplishments.json (if present), replaces onboard_text,
// preserves created_stories, and writes atomically via temp file + rename.
func (s *Service) writeAccomplishments(ctx context.Context, text string) error {
	path := filepath.Join(s.dataDir, "accomplishments.json")

	var acc model.AccomplishmentsJSON
	data, err := os.ReadFile(path) // #nosec G304 -- dataDir is a trusted config value
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("read accomplishments.json: %w", err)
		}
		// File does not exist yet — start fresh.
	} else {
		if jsonErr := json.Unmarshal(data, &acc); jsonErr != nil {
			return fmt.Errorf("accomplishments.json is corrupt — run go-apply onboard --reset: %w", jsonErr)
		}
		if acc.SchemaVersion != model.AccomplishmentsSchemaV1 {
			return fmt.Errorf("accomplishments.json schema_version %q is not supported — run go-apply onboard --reset", acc.SchemaVersion)
		}
	}

	acc.SchemaVersion = model.AccomplishmentsSchemaV1
	acc.OnboardText = text

	out, err := json.Marshal(acc)
	if err != nil {
		return fmt.Errorf("marshal accomplishments: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil { // #nosec G306
		return fmt.Errorf("write accomplishments tmp: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }()
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename accomplishments: %w", err)
	}

	s.log.InfoContext(ctx, "onboard: accomplishments.json written", "path", path)
	return nil
}

// writeDocument writes text to path.
// Returns a non-empty warning string on failure so the caller can degrade gracefully.
func (s *Service) writeDocument(ctx context.Context, sourceDoc, text, path string) string {
	s.log.DebugContext(ctx, "onboard: write start", "source", sourceDoc, "path", path, "bytes", len(text))
	if err := os.WriteFile(path, []byte(text), config.FilePerm); err != nil { // #nosec G306 -- user-owned data file
		msg := fmt.Sprintf("write %s: %v", path, err)
		s.log.WarnContext(ctx, "onboard: write failed", "source", sourceDoc, "error", err)
		return msg
	}
	s.log.InfoContext(ctx, "onboard: stored document", "source", sourceDoc, "path", path)
	return ""
}

// countSkillItems counts individual skill tokens in a skills document.
// Lines starting with # are treated as headings and skipped.
// Remaining lines are split on commas so "Go, Python, Docker" counts as 3.
func countSkillItems(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, token := range strings.Split(trimmed, ",") {
			if strings.TrimSpace(token) != "" {
				count++
			}
		}
	}
	return count
}

// validateLabel rejects empty labels and labels containing path separators.
// This prevents path traversal when the label is used to construct a filename.
func validateLabel(label string) error {
	if label == "" {
		return fmt.Errorf("label must not be empty")
	}
	if strings.ContainsAny(label, "/\\") {
		return fmt.Errorf("label must not contain path separators")
	}
	return nil
}
