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
			s.log.DebugContext(ctx, "decision",
				slog.String("name", "onboard.resume"),
				slog.String("chosen", "skip"),
				slog.String("reason", err.Error()),
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
		s.log.DebugContext(ctx, "decision",
			slog.String("name", "onboard.skills"),
			slog.String("chosen", "skip"),
			slog.String("reason", "empty"),
		)
	}

	if input.AccomplishmentsText != "" {
		sections := splitAccomplishmentSections(input.AccomplishmentsText)
		s.log.DebugContext(ctx, "onboard: storing accomplishments", "sections", len(sections))
		for i, section := range sections {
			sourceDoc := fmt.Sprintf("accomplishments:%d", i)
			path := filepath.Join(inputsDir, fmt.Sprintf("accomplishments-%d.md", i))
			if warn := s.storeDocument(ctx, sourceDoc, section, path); warn != "" {
				result.Warnings = append(result.Warnings, model.RiskWarning{Severity: model.SeverityWarn, Message: warn})
				continue
			}
			result.Stored = append(result.Stored, sourceDoc)
		}
		result.Summary.AccomplishmentsCount = len(sections)
		s.log.DebugContext(ctx, "onboard: accomplishments stored", "chunks", result.Summary.AccomplishmentsCount)
	} else {
		s.log.DebugContext(ctx, "decision",
			slog.String("name", "onboard.accomplishments"),
			slog.String("chosen", "skip"),
			slog.String("reason", "empty"),
		)
	}

	result.Summary.SkillsCount = countSkillItems(input.SkillsText)
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

// splitAccomplishmentSections splits accomplishments text into per-accomplishment chunks.
//
// Strategy:
//  1. If the text contains any markdown headings (lines starting with #), split on those.
//  2. Otherwise treat each blank-line-delimited paragraph as its own chunk — this handles
//     plain-text STAR-format files (Situation / Behavior / Impact paragraphs).
func splitAccomplishmentSections(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	// Check for markdown headings.
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			return splitOnHeadings(trimmed)
		}
	}

	// No headings — split on blank lines (paragraph-per-accomplishment).
	return splitOnParagraphs(trimmed)
}

// splitOnHeadings splits text each time a line starting with # is encountered.
func splitOnHeadings(text string) []string {
	lines := strings.Split(text, "\n")
	var sections []string
	var current strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") && current.Len() > 0 {
			if s := strings.TrimSpace(current.String()); s != "" {
				sections = append(sections, s)
			}
			current.Reset()
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		sections = append(sections, s)
	}
	return sections
}

// splitOnParagraphs splits on blank lines, returning each non-empty paragraph as a chunk.
func splitOnParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var sections []string
	for _, p := range raw {
		if s := strings.TrimSpace(p); s != "" {
			sections = append(sections, s)
		}
	}
	return sections
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
