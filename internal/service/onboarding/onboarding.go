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
			result.Warnings = append(result.Warnings, model.RiskWarning{
				Severity: "error",
				Message:  fmt.Sprintf("resume %q: %v", resume.Label, err),
			})
			continue
		}
		sourceDoc := "resume:" + resume.Label
		path := filepath.Join(inputsDir, resume.Label+".txt")
		if warn := s.storeDocument(ctx, sourceDoc, resume.Text, path); warn != "" {
			result.Warnings = append(result.Warnings, model.RiskWarning{Severity: "warn", Message: warn})
			continue
		}
		result.Stored = append(result.Stored, sourceDoc)
	}

	if input.SkillsText != "" {
		path := filepath.Join(inputsDir, "skills.md")
		if warn := s.storeDocument(ctx, "ref:skills", input.SkillsText, path); warn != "" {
			result.Warnings = append(result.Warnings, model.RiskWarning{Severity: "warn", Message: warn})
		} else {
			result.Stored = append(result.Stored, "ref:skills")
		}
	}

	if input.AccomplishmentsText != "" {
		path := filepath.Join(inputsDir, "accomplishments.md")
		if warn := s.storeDocument(ctx, "accomplishments", input.AccomplishmentsText, path); warn != "" {
			result.Warnings = append(result.Warnings, model.RiskWarning{Severity: "warn", Message: warn})
		} else {
			result.Stored = append(result.Stored, "accomplishments")
		}
	}

	return result, nil
}

// storeDocument writes text to path and upserts it into the profile repository.
// Returns a non-empty warning string on failure so the caller can degrade gracefully.
func (s *Service) storeDocument(ctx context.Context, sourceDoc, text, path string) string {
	if err := os.WriteFile(path, []byte(text), config.FilePerm); err != nil { // #nosec G306 -- user-owned data file
		msg := fmt.Sprintf("write %s: %v", path, err)
		s.log.WarnContext(ctx, "onboard: write failed", "source", sourceDoc, "error", err)
		return msg
	}

	vector, err := s.embedder.Embed(ctx, text)
	if err != nil {
		msg := fmt.Sprintf("embed %s: %v", sourceDoc, err)
		s.log.WarnContext(ctx, "onboard: embed failed", "source", sourceDoc, "error", err)
		return msg
	}

	if err := s.profile.UpsertDocument(ctx, sourceDoc, text, vector); err != nil {
		msg := fmt.Sprintf("upsert %s: %v", sourceDoc, err)
		s.log.WarnContext(ctx, "onboard: upsert failed", "source", sourceDoc, "error", err)
		return msg
	}

	s.log.InfoContext(ctx, "onboard: stored document", "source", sourceDoc, "path", path)
	return ""
}

// validateLabel rejects labels that contain path separators.
// This prevents path traversal when the label is used to construct a filename.
func validateLabel(label string) error {
	if strings.ContainsAny(label, "/\\") {
		return fmt.Errorf("label must not contain path separators")
	}
	if label == "" {
		return fmt.Errorf("label must not be empty")
	}
	return nil
}
