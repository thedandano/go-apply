package pdfrender_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pdfrender"
)

var _ port.PDFRenderer = (*pdfrender.Service)(nil)

func TestRenderPDF_NilInput_ReturnsError(t *testing.T) {
	svc := pdfrender.New()
	got, err := svc.RenderPDF(nil)
	if err == nil {
		t.Error("expected error for nil input, got nil")
	}
	if got != nil {
		t.Errorf("expected nil bytes for nil input, got %d bytes", len(got))
	}
}

func TestRenderPDF_EmptyContact_ReturnsValidBytes(t *testing.T) {
	sm := model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Jane Doe"},
	}
	svc := pdfrender.New()
	got, err := svc.RenderPDF(&sm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected non-empty PDF bytes, got 0")
	}
	if !bytes.HasPrefix(got, []byte("%PDF")) {
		t.Errorf("expected PDF magic bytes %%PDF, got: %q", got[:min(4, len(got))])
	}
}

func TestRenderPDF_AllSections_GoldenExtractedText(t *testing.T) {
	sm := model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact: model.ContactInfo{
			Name:     "Alice Engineer",
			Email:    "alice@example.com",
			Phone:    "555-0100",
			Location: "San Francisco, CA",
			Links:    []string{"linkedin.com/in/alice", "github.com/alice"},
		},
		Summary: "Seasoned software engineer with 10+ years building distributed systems.",
		Experience: []model.ExperienceEntry{
			{
				Company:   "BigCo",
				Role:      "Staff Engineer",
				StartDate: "2019-03",
				EndDate:   "Present",
				Location:  "Remote",
				Bullets: []string{
					"Led migration of monolith to microservices, reducing p99 latency by 45%.",
					"Mentored 8 engineers across 3 time zones.",
				},
			},
		},
		Education: []model.EducationEntry{
			{
				School:  "State University",
				Degree:  "BSc Computer Science",
				EndDate: "2013-05",
			},
		},
		Skills: &model.SkillsSection{
			Kind: model.SkillsKindCategorized,
			Categorized: map[string][]string{
				"Languages": {"Go", "Python", "TypeScript"},
				"Cloud":     {"AWS", "GCP", "Kubernetes"},
			},
		},
		Projects: []model.ProjectEntry{
			{
				Name:        "go-apply",
				Description: "Job application CLI with LLM-powered resume tailoring.",
				URL:         "github.com/alice/go-apply",
			},
		},
		Certifications: []model.CertificationEntry{
			{Name: "AWS Certified Solutions Architect", Issuer: "Amazon", Date: "2022-06"},
		},
		Awards: []model.AwardEntry{
			{Title: "Hackathon Champion", Date: "2021", Details: "Best developer tool."},
		},
		Volunteer: []model.VolunteerEntry{
			{
				Org:     "Code for Good",
				Role:    "Mentor",
				Bullets: []string{"Taught Python to 20 high school students."},
			},
		},
		Publications: []model.PublicationEntry{
			{Title: "Consistency Models in Practice", Venue: "USENIX ATC", Date: "2020"},
		},
		Languages: []model.LanguageEntry{
			{Name: "English", Proficiency: "Native"},
			{Name: "Spanish", Proficiency: "Conversational"},
		},
		Speaking: []model.SpeakingEntry{
			{Title: "Building Reliable Pipelines", Event: "GopherCon", Date: "2023"},
		},
		OpenSource: []model.OpenSourceEntry{
			{Project: "go-apply", Role: "Author", URL: "github.com/alice/go-apply"},
		},
		Patents: []model.PatentEntry{
			{Title: "Distributed Cache Invalidation", Number: "US20230001234", Date: "2023"},
		},
		Interests: []model.InterestEntry{
			{Name: "Open Source"},
			{Name: "Rock Climbing"},
		},
		References: []model.ReferenceEntry{
			{Name: "Available upon request"},
		},
	}

	svc := pdfrender.New()
	pdfBytes, err := svc.RenderPDF(&sm)
	if err != nil {
		t.Fatalf("RenderPDF error: %v", err)
	}
	if len(pdfBytes) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}

	goldenPath := filepath.Join("testdata", "golden", "full_resume.txt")

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		extracted := extractTextForGolden(t, pdfBytes)
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("failed to create golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(extracted), 0o644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		return
	}

	goldenBytes, err := os.ReadFile(goldenPath)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("golden not yet generated — run UPDATE_GOLDEN=1")
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	extracted := extractTextForGolden(t, pdfBytes)
	if strings.TrimSpace(extracted) != strings.TrimSpace(string(goldenBytes)) {
		t.Errorf("PDF text does not match golden.\ngot:\n%s\nwant:\n%s", extracted, string(goldenBytes))
	}
}

func TestRenderPDF_InvalidUTF8_ReturnsError(t *testing.T) {
	sm := model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Bad\xff\xfeName"},
	}
	svc := pdfrender.New()
	_, err := svc.RenderPDF(&sm)
	if err == nil {
		t.Error("expected error for invalid UTF-8 in contact name, got nil")
	}
}

// TestRenderPDF_NonLatin1_ReturnsError verifies that valid UTF-8 strings containing
// runes outside Latin-1 (code point > 0xFF) are rejected before rendering because
// fpdf core fonts (Arial) silently drop them.
func TestRenderPDF_NonLatin1_ReturnsError(t *testing.T) {
	cases := []struct {
		name  string
		field string
		value string
	}{
		{"CJK in contact name", "contact.name", "Alice 中文"},
		{"CJK in experience company", "experience[0].company", "Acme 日本語"},
		{"CJK in education school", "education[0].school", "MIT 東京"},
		{"CJK in skills flat", "skills.flat", "Go, Rust, 中文"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sm model.SectionMap
			sm.SchemaVersion = model.CurrentSchemaVersion
			switch tc.field {
			case "contact.name":
				sm.Contact = model.ContactInfo{Name: tc.value}
			case "experience[0].company":
				sm.Contact = model.ContactInfo{Name: "Alice"}
				sm.Experience = []model.ExperienceEntry{{Company: tc.value, Role: "Eng", StartDate: "2020-01", Bullets: []string{"built things"}}}
			case "education[0].school":
				sm.Contact = model.ContactInfo{Name: "Alice"}
				sm.Education = []model.EducationEntry{{School: tc.value, Degree: "BS CS"}}
			case "skills.flat":
				sm.Contact = model.ContactInfo{Name: "Alice"}
				sm.Skills = &model.SkillsSection{Kind: model.SkillsKindFlat, Flat: tc.value}
			}
			svc := pdfrender.New()
			_, err := svc.RenderPDF(&sm)
			if err == nil {
				t.Errorf("expected error for non-Latin-1 rune in %s, got nil", tc.field)
			}
		})
	}
}

func TestRenderPDF_LogsRenderEvents(t *testing.T) {
	origLogger := slog.Default()
	h := &testLogHandler{}
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(origLogger)

	sm := model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Log Test"},
	}
	svc := pdfrender.New()
	_, err := svc.RenderPDF(&sm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !h.hasMsg("pdfrender.render") {
		t.Error("expected slog record with message \"pdfrender.render\" to be emitted")
	}
	if !h.hasMsg("pdfrender.done") {
		t.Error("expected slog record with message \"pdfrender.done\" to be emitted")
	}
}

// extractTextForGolden extracts plain text from PDF bytes using the pdftotext CLI tool.
// It skips the test if pdftotext is not installed.
func extractTextForGolden(t *testing.T, pdfBytes []byte) string {
	t.Helper()
	path, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed — skipping golden extraction")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-", "-")
	cmd.Stdin = bytes.NewReader(pdfBytes)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pdftotext failed: %v", err)
	}
	return string(out)
}

type testLogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *testLogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *testLogHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // slog.Handler interface requires value receiver
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *testLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *testLogHandler) WithGroup(_ string) slog.Handler      { return h }
func (h *testLogHandler) hasMsg(msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.records {
		if h.records[i].Message == msg {
			return true
		}
	}
	return false
}
