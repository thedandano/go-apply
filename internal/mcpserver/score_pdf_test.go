package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	extractSvc "github.com/thedandano/go-apply/internal/service/extract"
	pdfrender "github.com/thedandano/go-apply/internal/service/pdfrender"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	scorerSvc "github.com/thedandano/go-apply/internal/service/scorer"
)

// ── inline stubs for scoreSectionsPDF tests ───────────────────────────────────

// pdfScorerStub scores any input with a fixed result.
type pdfScorerStub struct {
	result model.ScoreResult
	err    error
}

func (s *pdfScorerStub) Score(_ *model.ScorerInput) (model.ScoreResult, error) {
	return s.result, s.err
}

// pdfRendererStub renders sections to fixed bytes.
type pdfRendererStub struct {
	data []byte
	err  error
}

func (s *pdfRendererStub) RenderPDF(_ *model.SectionMap) ([]byte, error) {
	return s.data, s.err
}

// pdfExtractorStub extracts fixed text from any bytes.
type pdfExtractorStub struct {
	text string
	err  error
}

func (s *pdfExtractorStub) Extract(_ context.Context, _ []byte) (string, error) {
	return s.text, s.err
}

// capturingLogHandler records slog records so tests can assert on structured fields.
type capturingLogHandler struct {
	records []slog.Record
}

func (h *capturingLogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *capturingLogHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic
	h.records = append(h.records, r)
	return nil
}
func (h *capturingLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingLogHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *capturingLogHandler) hasSessionID(sessionID string) bool {
	for i := range h.records {
		found := false
		h.records[i].Attrs(func(a slog.Attr) bool {
			if a.Key == "session_id" && a.Value.String() == sessionID {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func (h *capturingLogHandler) hasMsgContaining(sub string) bool {
	for i := range h.records {
		if strings.Contains(h.records[i].Message, sub) {
			return true
		}
	}
	return false
}

func minimalSections() *model.SectionMap {
	return &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Alice"},
		Experience: []model.ExperienceEntry{
			{Company: "Acme", Role: "Engineer", StartDate: "2020-01", Bullets: []string{"Built systems"}},
		},
	}
}

func minimalJD() *model.JDData {
	return &model.JDData{
		Title:    "Go Engineer",
		Company:  "Acme",
		Required: []string{"go"},
	}
}

// ── T008 tests ────────────────────────────────────────────────────────────────

// T008 happy path: render/extract/score log entries emitted with session_id;
// non-zero ScoreResult returned.
func TestScoreSectionsPDF_HappyPath_ReturnsScore(t *testing.T) {
	lh := &capturingLogHandler{}
	orig := slog.Default()
	slog.SetDefault(slog.New(lh))
	t.Cleanup(func() { slog.SetDefault(orig) })

	sessionID := "test-session-001"
	expectedTotal := 45.0

	deps := &pipeline.ApplyConfig{
		PDFRenderer: &pdfRendererStub{data: []byte("%PDF-1.4 fake"), err: nil},
		Extractor:   &pdfExtractorStub{text: "extracted resume text", err: nil},
		Scorer: &pdfScorerStub{result: model.ScoreResult{
			Breakdown: model.ScoreBreakdown{KeywordMatch: 45.0},
		}},
	}

	result, err := scoreSectionsPDF(
		context.Background(),
		minimalSections(),
		"test-resume",
		sessionID,
		minimalJD(),
		&config.Config{},
		deps,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Breakdown.Total() != expectedTotal {
		t.Errorf("Total() = %v, want %v", result.Breakdown.Total(), expectedTotal)
	}

	// Verify session_id was present in at least one log record.
	if !lh.hasSessionID(sessionID) {
		t.Error("expected at least one log entry with session_id attribute")
	}
	// Verify render, extract, score log events were emitted.
	for _, msg := range []string{"score_pdf.render", "score_pdf.extract", "score_pdf.score"} {
		if !lh.hasMsgContaining(msg) {
			t.Errorf("expected log entry with message %q", msg)
		}
	}
}

// T008: empty extracted text returns error containing resume label.
func TestScoreSectionsPDF_EmptyExtractedText_ReturnsError(t *testing.T) {
	deps := &pipeline.ApplyConfig{
		PDFRenderer: &pdfRendererStub{data: []byte("%PDF-1.4 fake"), err: nil},
		Extractor:   &pdfExtractorStub{text: "", err: nil}, // empty, no error from extractor
		Scorer:      &pdfScorerStub{},
	}

	const label = "my-resume"
	_, err := scoreSectionsPDF(
		context.Background(),
		minimalSections(),
		label,
		"sess-002",
		minimalJD(),
		&config.Config{},
		deps,
	)
	if err == nil {
		t.Fatal("expected error for empty extracted text, got nil")
	}
	if !strings.Contains(err.Error(), label) {
		t.Errorf("error %q must contain resume label %q", err.Error(), label)
	}
}

// T008: extraction failure propagates.
func TestScoreSectionsPDF_ExtractError_Propagates(t *testing.T) {
	extractErr := fmt.Errorf("simulated extract failure")
	deps := &pipeline.ApplyConfig{
		PDFRenderer: &pdfRendererStub{data: []byte("%PDF-1.4 fake"), err: nil},
		Extractor:   &pdfExtractorStub{text: "", err: extractErr},
		Scorer:      &pdfScorerStub{},
	}

	_, err := scoreSectionsPDF(
		context.Background(),
		minimalSections(),
		"any-label",
		"sess-003",
		minimalJD(),
		&config.Config{},
		deps,
	)
	if err == nil {
		t.Fatal("expected error from extraction failure, got nil")
	}
}

// T008: render failure returns error.
func TestScoreSectionsPDF_RenderError_Propagates(t *testing.T) {
	renderErr := fmt.Errorf("simulated render failure")
	deps := &pipeline.ApplyConfig{
		PDFRenderer: &pdfRendererStub{data: nil, err: renderErr},
		Extractor:   &pdfExtractorStub{text: "text", err: nil},
		Scorer:      &pdfScorerStub{},
	}

	_, err := scoreSectionsPDF(
		context.Background(),
		minimalSections(),
		"any-label",
		"sess-004",
		minimalJD(),
		&config.Config{},
		deps,
	)
	if err == nil {
		t.Fatal("expected error from render failure, got nil")
	}
}

// T015: routing regression test — NextActionAfterT1 routing is preserved via the
// PDF path (SC-004: routing decisions preserved after extractor swap).
// Uses real pdfrender + extract + scorer deps to verify routing is deterministic:
// calling scoreSectionsPDF twice with the same input must produce the same
// next_action decision.
func TestRoutingDecision_PreservedWithPDFPath(t *testing.T) {
	defaults := config.EmbeddedDefaults()
	deps := &pipeline.ApplyConfig{
		PDFRenderer: pdfrender.New(),
		Extractor:   extractSvc.New(),
		Scorer:      scorerSvc.New(defaults),
		Defaults:    defaults,
	}

	sections := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Alice"},
		Skills: &model.SkillsSection{
			Kind: model.SkillsKindCategorized,
			Categorized: map[string][]string{
				"Languages": {"Go", "Python"},
				"Tools":     {"Kubernetes", "Docker"},
			},
		},
		Experience: []model.ExperienceEntry{
			{
				Company:   "Acme",
				Role:      "Software Engineer",
				StartDate: "2020-01",
				EndDate:   "2023-01",
				Bullets:   []string{"Built Go microservices", "Deployed on Kubernetes"},
			},
		},
	}
	jd := &model.JDData{
		Title:    "Go Engineer",
		Company:  "Acme",
		Required: []string{"go", "kubernetes"},
	}

	// Call scoreSectionsPDF twice with identical input — routing must be stable.
	r1, err := scoreSectionsPDF(context.Background(), sections, "resume-a", "sess-t015-1", jd, &config.Config{}, deps)
	if err != nil {
		t.Fatalf("first scoreSectionsPDF: %v", err)
	}
	r2, err := scoreSectionsPDF(context.Background(), sections, "resume-a", "sess-t015-2", jd, &config.Config{}, deps)
	if err != nil {
		t.Fatalf("second scoreSectionsPDF: %v", err)
	}

	action1 := NextActionAfterT1(r1.Breakdown.Total())
	action2 := NextActionAfterT1(r2.Breakdown.Total())

	if action1 != action2 {
		t.Errorf("routing is non-deterministic: call 1 = %q (score %.2f), call 2 = %q (score %.2f)",
			action1, r1.Breakdown.Total(), action2, r2.Breakdown.Total())
	}
	// Routing must be one of the valid terminal actions.
	if action1 != "tailor_t2" && action1 != "cover_letter" {
		t.Errorf("NextActionAfterT1 returned unexpected action %q for score %.2f", action1, r1.Breakdown.Total())
	}
}
