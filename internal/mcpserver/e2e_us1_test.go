package mcpserver_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	fsrepo "github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/tailor"
)

// TestUS1_StructuredEdits_E2E verifies that ApplyEdits works correctly through
// a real sections file round-trip regardless of resume heading style.
//
// US1: "T1 tailoring works regardless of resume heading style."
// ApplyEdits targets sections by structural key ("skills", "experience"),
// not by the heading text stored in SkillsSection.Kind. Non-standard headings
// such as "Technical Stack" must not cause rejections.
func TestUS1_StructuredEdits_E2E(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("round_trip_and_edits", func(t *testing.T) {
		dataDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dataDir, "inputs"), 0o755); err != nil {
			t.Fatalf("MkdirAll inputs: %v", err)
		}

		sm := model.SectionMap{
			SchemaVersion: model.CurrentSchemaVersion,
			Contact:       model.ContactInfo{Name: "Jane Doe"},
			Skills: &model.SkillsSection{
				Kind: model.SkillsKindFlat,
				Flat: "Go, Python",
			},
			Experience: []model.ExperienceEntry{
				{
					Company:   "Acme Corp",
					Role:      "Software Engineer",
					StartDate: "2022-01",
					Bullets: []string{
						"Designed REST APIs serving 10k rps",
						"Reduced deployment time by 40%",
					},
				},
			},
		}

		repo := fsrepo.NewResumeRepository(dataDir)

		if err := repo.SaveSections("myresume", sm); err != nil {
			t.Fatalf("SaveSections: %v", err)
		}

		loaded, err := repo.LoadSections("myresume")
		if err != nil {
			t.Fatalf("LoadSections: %v", err)
		}
		if loaded.Skills == nil {
			t.Fatal("loaded.Skills is nil after round-trip")
		}
		if loaded.Skills.Flat != "Go, Python" {
			t.Errorf("loaded.Skills.Flat = %q, want %q", loaded.Skills.Flat, "Go, Python")
		}
		if len(loaded.Experience) == 0 {
			t.Fatal("loaded.Experience is empty after round-trip")
		}
		if len(loaded.Experience[0].Bullets) != 2 {
			t.Errorf("loaded.Experience[0].Bullets len = %d, want 2", len(loaded.Experience[0].Bullets))
		}

		svc := tailor.New(nil, log)
		edits := []port.Edit{
			{Section: "skills", Op: port.EditOpReplace, Value: "Go, Python, Kubernetes"},
			{Section: "experience", Op: port.EditOpReplace, Target: "exp-0-b0", Value: "Led K8s migration for 50-service platform"},
		}
		result, err := svc.ApplyEdits(context.Background(), loaded, edits)
		if err != nil {
			t.Fatalf("ApplyEdits: %v", err)
		}

		if len(result.EditsApplied) != 2 {
			t.Errorf("EditsApplied = %d, want 2", len(result.EditsApplied))
		}
		if len(result.EditsRejected) != 0 {
			t.Errorf("EditsRejected = %d, want 0 — rejections: %+v", len(result.EditsRejected), result.EditsRejected)
		}
		if result.NewSections.Skills == nil {
			t.Fatal("result.NewSections.Skills is nil")
		}
		if result.NewSections.Skills.Flat != "Go, Python, Kubernetes" {
			t.Errorf("Skills.Flat = %q, want %q", result.NewSections.Skills.Flat, "Go, Python, Kubernetes")
		}
		if len(result.NewSections.Experience) == 0 {
			t.Fatal("result.NewSections.Experience is empty")
		}
		if result.NewSections.Experience[0].Bullets[0] != "Led K8s migration for 50-service platform" {
			t.Errorf("Bullets[0] = %q, want %q",
				result.NewSections.Experience[0].Bullets[0],
				"Led K8s migration for 50-service platform")
		}
	})

	// nonstandard_heading_kind: Skills.Kind = "Technical Stack" round-trips through the
	// sections file and ApplyEdits still accepts edits without rejection — the edit envelope
	// targets the section key "skills" structurally, not by heading text.
	t.Run("nonstandard_heading_kind", func(t *testing.T) {
		dataDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dataDir, "inputs"), 0o755); err != nil {
			t.Fatalf("MkdirAll inputs: %v", err)
		}

		sm := model.SectionMap{
			SchemaVersion: model.CurrentSchemaVersion,
			Contact:       model.ContactInfo{Name: "John Smith"},
			Skills: &model.SkillsSection{
				Kind: model.SkillsKind("Technical Stack"),
				Flat: "Go, Python",
			},
			Experience: []model.ExperienceEntry{
				{
					Company:   "Globex",
					Role:      "Platform Engineer",
					StartDate: "2023-06",
					Bullets: []string{
						"Built observability stack",
						"Owned on-call rotation for 12 services",
					},
				},
			},
		}

		repo := fsrepo.NewResumeRepository(dataDir)
		if err := repo.SaveSections("myresume", sm); err != nil {
			t.Fatalf("SaveSections: %v", err)
		}
		loaded, err := repo.LoadSections("myresume")
		if err != nil {
			t.Fatalf("LoadSections: %v", err)
		}

		svc := tailor.New(nil, log)
		edits := []port.Edit{
			{Section: "skills", Op: port.EditOpReplace, Value: "Go, Python, Kubernetes"},
			{Section: "experience", Op: port.EditOpReplace, Target: "exp-0-b0", Value: "Led K8s migration for 50-service platform"},
		}
		result, err := svc.ApplyEdits(context.Background(), loaded, edits)
		if err != nil {
			t.Fatalf("ApplyEdits: %v", err)
		}

		if len(result.EditsApplied) != 2 {
			t.Errorf("EditsApplied = %d, want 2 (non-standard heading must not block structural edits)", len(result.EditsApplied))
		}
		if len(result.EditsRejected) != 0 {
			t.Errorf("EditsRejected = %d, want 0 — rejections: %+v", len(result.EditsRejected), result.EditsRejected)
		}
		if result.NewSections.Skills == nil {
			t.Fatal("result.NewSections.Skills is nil")
		}
		if result.NewSections.Skills.Kind != model.SkillsKind("Technical Stack") {
			t.Errorf("Skills.Kind = %q, want %q (Kind must be preserved through structural edit)",
				result.NewSections.Skills.Kind, "Technical Stack")
		}
		if result.NewSections.Skills.Flat != "Go, Python, Kubernetes" {
			t.Errorf("Skills.Flat = %q, want %q", result.NewSections.Skills.Flat, "Go, Python, Kubernetes")
		}
		if len(result.NewSections.Experience) == 0 {
			t.Fatal("result.NewSections.Experience is empty")
		}
		if result.NewSections.Experience[0].Bullets[0] != "Led K8s migration for 50-service platform" {
			t.Errorf("Bullets[0] = %q, want %q",
				result.NewSections.Experience[0].Bullets[0],
				"Led K8s migration for 50-service platform")
		}
	})
}
