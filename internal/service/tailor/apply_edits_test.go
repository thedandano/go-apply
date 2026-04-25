package tailor_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/tailor"
)

func newBaseSection() model.SectionMap {
	return model.SectionMap{
		Skills: &model.SkillsSection{
			Kind: model.SkillsKindFlat,
			Flat: "Go, Python",
		},
		Experience: []model.ExperienceEntry{
			{
				Company: "Acme Corp",
				Role:    "Engineer",
				Bullets: []string{
					"Designed REST APIs serving 10k rps",
					"Migrated legacy monolith to microservices",
					"Reduced deployment time by 40%",
				},
			},
		},
	}
}

func TestApplyEdits(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := tailor.New(nil, nil, log)
	ctx := context.Background()

	tests := []struct {
		name         string
		sections     func() model.SectionMap
		edits        []port.Edit
		wantApplied  int
		wantRejected int
		check        func(t *testing.T, result port.EditResult)
	}{
		{
			name:     "skills flat replace",
			sections: newBaseSection,
			edits: []port.Edit{
				{Section: "skills", Op: port.EditOpReplace, Value: "Go, Python, Rust"},
			},
			wantApplied:  1,
			wantRejected: 0,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if result.NewSections.Skills == nil {
					t.Fatal("NewSections.Skills is nil")
				}
				if result.NewSections.Skills.Flat != "Go, Python, Rust" {
					t.Errorf("Skills.Flat = %q, want %q", result.NewSections.Skills.Flat, "Go, Python, Rust")
				}
			},
		},
		{
			name:     "skills add",
			sections: newBaseSection,
			edits: []port.Edit{
				{Section: "skills", Op: port.EditOpAdd, Value: "Docker"},
			},
			wantApplied:  1,
			wantRejected: 0,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if result.NewSections.Skills == nil {
					t.Fatal("NewSections.Skills is nil")
				}
				if !strings.Contains(result.NewSections.Skills.Flat, "Docker") {
					t.Errorf("Skills.Flat %q does not contain %q after add", result.NewSections.Skills.Flat, "Docker")
				}
			},
		},
		{
			name:     "experience bullet replace",
			sections: newBaseSection,
			edits: []port.Edit{
				{Section: "experience", Op: port.EditOpReplace, Target: "exp-0-b1", Value: "Led migration to K8s"},
			},
			wantApplied:  1,
			wantRejected: 0,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if len(result.NewSections.Experience) == 0 {
					t.Fatal("NewSections.Experience is empty")
				}
				bullets := result.NewSections.Experience[0].Bullets
				if len(bullets) < 2 {
					t.Fatalf("expected at least 2 bullets, got %d", len(bullets))
				}
				if bullets[1] != "Led migration to K8s" {
					t.Errorf("Bullets[1] = %q, want %q", bullets[1], "Led migration to K8s")
				}
			},
		},
		{
			name:     "experience bullet remove",
			sections: newBaseSection,
			edits: []port.Edit{
				{Section: "experience", Op: port.EditOpRemove, Target: "exp-0-b0"},
			},
			wantApplied:  1,
			wantRejected: 0,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if len(result.NewSections.Experience) == 0 {
					t.Fatal("NewSections.Experience is empty")
				}
				bullets := result.NewSections.Experience[0].Bullets
				if len(bullets) != 2 {
					t.Errorf("expected 2 bullets after remove, got %d", len(bullets))
				}
			},
		},
		{
			name:     "invalid target rejected",
			sections: newBaseSection,
			edits: []port.Edit{
				{Section: "experience", Op: port.EditOpReplace, Target: "exp-99-b0", Value: "x"},
			},
			wantApplied:  0,
			wantRejected: 1,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if len(result.EditsRejected) == 0 {
					t.Fatal("expected a rejection, got none")
				}
				if result.EditsRejected[0].Reason == "" {
					t.Error("rejection Reason must not be empty")
				}
			},
		},
		{
			name:     "unknown section rejected",
			sections: newBaseSection,
			edits: []port.Edit{
				{Section: "unknown_section", Op: port.EditOpAdd, Value: "x"},
			},
			wantApplied:  0,
			wantRejected: 1,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if len(result.EditsRejected) == 0 {
					t.Fatal("expected a rejection for unknown section, got none")
				}
				if result.EditsRejected[0].Reason == "" {
					t.Error("rejection Reason must not be empty")
				}
			},
		},
		{
			name:     "mixed batch",
			sections: newBaseSection,
			edits: []port.Edit{
				{Section: "skills", Op: port.EditOpReplace, Value: "Go, Rust"},
				{Section: "experience", Op: port.EditOpReplace, Target: "exp-0-b0", Value: "Owned platform reliability"},
				{Section: "experience", Op: port.EditOpReplace, Target: "exp-99-b0", Value: "invalid"},
			},
			wantApplied:  2,
			wantRejected: 1,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if result.NewSections.Skills == nil {
					t.Fatal("NewSections.Skills is nil")
				}
				if result.NewSections.Skills.Flat != "Go, Rust" {
					t.Errorf("Skills.Flat = %q, want %q", result.NewSections.Skills.Flat, "Go, Rust")
				}
				if len(result.NewSections.Experience) == 0 {
					t.Fatal("NewSections.Experience is empty")
				}
				if result.NewSections.Experience[0].Bullets[0] != "Owned platform reliability" {
					t.Errorf("Bullets[0] = %q, want %q",
						result.NewSections.Experience[0].Bullets[0], "Owned platform reliability")
				}
				if len(result.EditsRejected) == 0 {
					t.Fatal("expected 1 rejection, got none")
				}
				if result.EditsRejected[0].Index != 2 {
					t.Errorf("rejected edit Index = %d, want 2", result.EditsRejected[0].Index)
				}
			},
		},
		{
			name:         "empty edits",
			sections:     newBaseSection,
			edits:        []port.Edit{},
			wantApplied:  0,
			wantRejected: 0,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if result.NewSections.Skills == nil {
					t.Fatal("NewSections.Skills is nil")
				}
				if result.NewSections.Skills.Flat != "Go, Python" {
					t.Errorf("Skills.Flat = %q, want %q (unchanged)", result.NewSections.Skills.Flat, "Go, Python")
				}
			},
		},
	}

	// Categorized kind — skills edits must be rejected (not corrupt the discriminated union).
	tests = append(tests, struct {
		name         string
		sections     func() model.SectionMap
		edits        []port.Edit
		wantApplied  int
		wantRejected int
		check        func(t *testing.T, result port.EditResult)
	}{
		name: "skills categorized kind rejects flat ops",
		sections: func() model.SectionMap {
			return model.SectionMap{
				Skills: &model.SkillsSection{
					Kind:        model.SkillsKindCategorized,
					Categorized: map[string][]string{"Cloud": {"AWS", "GCP"}},
				},
			}
		},
		edits: []port.Edit{
			{Section: "skills", Op: port.EditOpReplace, Value: "AWS, GCP, Azure"},
		},
		wantApplied:  0,
		wantRejected: 1,
		check: func(t *testing.T, result port.EditResult) {
			t.Helper()
			if len(result.EditsRejected) == 0 {
				t.Fatal("expected rejection for Categorized kind, got none")
			}
			if result.EditsRejected[0].Reason == "" {
				t.Error("rejection Reason must not be empty")
			}
			// Categorized map must be unmodified.
			cats := result.NewSections.Skills.Categorized
			if items, ok := cats["Cloud"]; !ok || len(items) != 2 {
				t.Errorf("Categorized map was mutated: Cloud = %v", cats["Cloud"])
			}
			// Flat must remain empty — discriminated union must not be corrupted.
			if result.NewSections.Skills.Flat != "" {
				t.Errorf("Skills.Flat must remain empty for Categorized kind, got %q", result.NewSections.Skills.Flat)
			}
		},
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sections := tc.sections()
			result, err := svc.ApplyEdits(ctx, sections, tc.edits)
			if err != nil {
				t.Fatalf("ApplyEdits returned error: %v", err)
			}

			if len(result.EditsApplied) != tc.wantApplied {
				t.Errorf("EditsApplied count = %d, want %d", len(result.EditsApplied), tc.wantApplied)
			}
			if len(result.EditsRejected) != tc.wantRejected {
				t.Errorf("EditsRejected count = %d, want %d", len(result.EditsRejected), tc.wantRejected)
			}

			tc.check(t, result)
		})
	}
}
