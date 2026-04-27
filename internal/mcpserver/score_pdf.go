package mcpserver

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// ScoringMethodPDFExtracted is the scoring_method value set when a score was
// computed from PDF-extracted text via the render→extract→score pipeline.
const ScoringMethodPDFExtracted = "pdf_extracted"

// scoreSectionsPDF renders sections to PDF, extracts plain text, and scores it.
// Called by T0/T1/T2 handlers.
func scoreSectionsPDF(
	ctx context.Context,
	sections *model.SectionMap,
	label string,
	sessionID string,
	jd *model.JDData,
	cfg *config.Config,
	deps *pipeline.ApplyConfig,
) (model.ScoreResult, error) {
	// Render sections to PDF bytes.
	pdfBytes, err := deps.PDFRenderer.RenderPDF(sections)
	if err != nil {
		slog.ErrorContext(ctx, "score_pdf.failed",
			"session_id", sessionID, "label", label, "stage", "render", "error", err)
		return model.ScoreResult{}, fmt.Errorf("scoreSectionsPDF: render %s: %w", label, err)
	}
	slog.InfoContext(ctx, "score_pdf.render",
		"session_id", sessionID, "label", label, "sections_count", countSectionsInMap(sections))

	// Extract plain text from PDF.
	text, err := deps.Extractor.Extract(ctx, pdfBytes)
	if err != nil {
		slog.ErrorContext(ctx, "score_pdf.failed",
			"session_id", sessionID, "label", label, "stage", "extract", "error", err)
		return model.ScoreResult{}, fmt.Errorf("scoreSectionsPDF: extract %s: %w", label, err)
	}
	if len(text) == 0 {
		emptyErr := fmt.Errorf("scoreSectionsPDF: empty extracted text for %s", label)
		slog.ErrorContext(ctx, "score_pdf.failed",
			"session_id", sessionID, "label", label, "stage", "extract", "error", emptyErr)
		return model.ScoreResult{}, emptyErr
	}
	slog.InfoContext(ctx, "score_pdf.extract",
		"session_id", sessionID, "label", label, "extracted_bytes", len(text))

	// Score using pipeline.ScoreResume to pick up seniority resolution.
	pl := pipeline.NewApplyPipeline(deps)
	result, err := pl.ScoreResume(ctx, text, label, jd, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "score_pdf.failed",
			"session_id", sessionID, "label", label, "stage", "score", "error", err)
		return model.ScoreResult{}, fmt.Errorf("scoreSectionsPDF: score %s: %w", label, err)
	}
	slog.InfoContext(ctx, "score_pdf.score",
		"session_id", sessionID, "label", label, "score_total", result.Breakdown.Total())

	return result, nil
}

// countSectionsInMap returns the number of non-nil populated top-level sections.
func countSectionsInMap(sections *model.SectionMap) int {
	if sections == nil {
		return 0
	}
	if len(sections.Order) > 0 {
		return len(sections.Order)
	}
	// Fall back to counting non-empty fields.
	n := 0
	if sections.Contact.Name != "" {
		n++
	}
	if sections.Summary != "" {
		n++
	}
	if len(sections.Experience) > 0 {
		n++
	}
	if sections.Skills != nil {
		n++
	}
	if len(sections.Education) > 0 {
		n++
	}
	if len(sections.Projects) > 0 {
		n++
	}
	if len(sections.Certifications) > 0 {
		n++
	}
	if len(sections.Awards) > 0 {
		n++
	}
	if len(sections.Volunteer) > 0 {
		n++
	}
	if len(sections.Publications) > 0 {
		n++
	}
	if len(sections.Languages) > 0 {
		n++
	}
	if len(sections.Speaking) > 0 {
		n++
	}
	if len(sections.OpenSource) > 0 {
		n++
	}
	if len(sections.Patents) > 0 {
		n++
	}
	if len(sections.Interests) > 0 {
		n++
	}
	if len(sections.References) > 0 {
		n++
	}
	return n
}
