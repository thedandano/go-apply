// Package onboarding stores resume, skills, and accomplishments text into the
// profile repository, embedding each document for later vector retrieval.
package onboarding

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Onboarder = (*Service)(nil)

// Service implements port.Onboarder.
// It writes text files to DataDir/inputs/, then embeds and upserts each into
// the profile repository. Individual embed/upsert failures degrade to Warnings.
type Service struct {
	profile  port.ProfileRepository
	embedder port.EmbeddingClient
	dataDir  string
	log      *slog.Logger
}

// New constructs a Service with the provided dependencies.
// dataDir should be config.DataDir().
func New(profile port.ProfileRepository, embedder port.EmbeddingClient, dataDir string, log *slog.Logger) *Service {
	return &Service{
		profile:  profile,
		embedder: embedder,
		dataDir:  dataDir,
		log:      log,
	}
}

// Run stores all provided resumes, skills, and accomplishments into the profile
// repository. Each document is written to disk and embedded for vector retrieval.
// Failures for individual documents are collected as Warnings; Run always returns nil error.
func (s *Service) Run(ctx context.Context, input model.OnboardInput) (model.OnboardResult, error) {
	var result model.OnboardResult

	inputsDir := filepath.Join(s.dataDir, "inputs")
	if err := os.MkdirAll(inputsDir, config.DirPerm); err != nil {
		return result, fmt.Errorf("create inputs dir: %w", err)
	}

	for _, resume := range input.Resumes {
		if err := validateLabel(resume.Label); err != nil {
			logger.Decision(ctx, s.log, "onboard.resume", "skip", err.Error(), slog.String("label", resume.Label))
			result.Warnings = append(result.Warnings, model.RiskWarning{
				Severity: model.SeverityError,
				Message:  fmt.Sprintf("resume %q: %v", resume.Label, err),
			})
			continue
		}
		sourceDoc := "resume:" + resume.Label
		path := filepath.Join(inputsDir, resume.Label+".txt")
		s.log.DebugContext(ctx, "onboard: storing resume", "source", sourceDoc, "input_bytes", len(resume.Text))
		if warn := s.storeDocument(ctx, sourceDoc, resume.Text, path); warn != "" {
			result.Warnings = append(result.Warnings, model.RiskWarning{Severity: model.SeverityWarn, Message: warn})
			continue
		}
		s.log.DebugContext(ctx, "onboard: resume stored", "source", sourceDoc)
		result.Stored = append(result.Stored, sourceDoc)
		result.Summary.ResumesAdded++
	}

	if input.SkillsText != "" {
		path := filepath.Join(inputsDir, "skills.md")
		s.log.DebugContext(ctx, "onboard: storing skills", "input_bytes", len(input.SkillsText))
		if warn := s.storeDocument(ctx, "ref:skills", input.SkillsText, path); warn != "" {
			result.Warnings = append(result.Warnings, model.RiskWarning{Severity: model.SeverityWarn, Message: warn})
		} else {
			s.log.DebugContext(ctx, "onboard: skills stored")
			result.Stored = append(result.Stored, "ref:skills")
		}
	} else {
		logger.Decision(ctx, s.log, "onboard.skills", "skip", "empty")
	}

	if input.AccomplishmentsText != "" {
		path := filepath.Join(inputsDir, "accomplishments.md")
		s.log.DebugContext(ctx, "onboard: storing accomplishments", "input_bytes", len(input.AccomplishmentsText))
		if warn := s.storeDocument(ctx, "accomplishments", input.AccomplishmentsText, path); warn != "" {
			result.Warnings = append(result.Warnings, model.RiskWarning{Severity: model.SeverityWarn, Message: warn})
		} else {
			s.log.DebugContext(ctx, "onboard: accomplishments stored")
			result.Stored = append(result.Stored, "accomplishments")
		}
	} else {
		logger.Decision(ctx, s.log, "onboard.accomplishments", "skip", "empty")
	}

	result.Summary.SkillsCount = countSkillItems(input.SkillsText)
	result.Summary.AccomplishmentsCount = countAccomplishmentSections(input.AccomplishmentsText)
	result.Summary.TotalChunks = len(result.Stored)

	return result, nil
}

// storeDocument writes text to path and upserts it into the profile repository.
// Returns a non-empty warning string on failure so the caller can degrade gracefully.
func (s *Service) storeDocument(ctx context.Context, sourceDoc, text, path string) string {
	s.log.DebugContext(ctx, "onboard: write start", "source", sourceDoc, "path", path, "bytes", len(text))
	if err := os.WriteFile(path, []byte(text), config.FilePerm); err != nil { // #nosec G306 -- user-owned data file
		msg := fmt.Sprintf("write %s: %v", path, err)
		s.log.WarnContext(ctx, "onboard: write failed", "source", sourceDoc, "error", err)
		return msg
	}
	s.log.DebugContext(ctx, "onboard: write end", "source", sourceDoc)

	s.log.DebugContext(ctx, "onboard: embed start", "source", sourceDoc, "bytes", len(text))
	vector, err := s.embedder.Embed(ctx, text)
	if err != nil {
		msg := fmt.Sprintf("embed %s: %v", sourceDoc, err)
		s.log.WarnContext(ctx, "onboard: embed failed", "source", sourceDoc, "error", err)
		return msg
	}
	s.log.DebugContext(ctx, "onboard: embed end", "source", sourceDoc, "vector_dim", len(vector))

	s.log.DebugContext(ctx, "onboard: upsert start", "source", sourceDoc)
	if err := s.profile.UpsertDocument(ctx, sourceDoc, text, vector); err != nil {
		msg := fmt.Sprintf("upsert %s: %v", sourceDoc, err)
		s.log.WarnContext(ctx, "onboard: upsert failed", "source", sourceDoc, "error", err)
		return msg
	}
	s.log.DebugContext(ctx, "onboard: upsert end", "source", sourceDoc)

	s.log.InfoContext(ctx, "onboard: stored document", "source", sourceDoc, "path", path)
	return ""
}

// countSkillItems counts non-empty, non-heading lines in a skills document.
// Each such line represents an individual skill entry.
func countSkillItems(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			count++
		}
	}
	return count
}

// countAccomplishmentSections counts ## headings in an accomplishments document.
// Each ## section represents a distinct accomplishment.
func countAccomplishmentSections(text string) int {
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			count++
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
