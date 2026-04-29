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
	svc := tailor.New(nil, log)
	ctx := context.Background()

	tests := []struct { //nolint:prealloc
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

	type catTest struct {
		name         string
		sections     func() model.SectionMap
		edits        []port.Edit
		wantApplied  int
		wantRejected int
		check        func(t *testing.T, result port.EditResult)
	}
	catTests := []catTest{
		{
			name: "skills categorized rejects ops with missing category",
			sections: func() model.SectionMap {
				return model.SectionMap{
					Skills: &model.SkillsSection{
						Kind:        model.SkillsKindCategorized,
						Categorized: map[string][]string{"Cloud": {"AWS", "GCP"}},
					},
				}
			},
			edits:        []port.Edit{{Section: "skills", Op: port.EditOpReplace, Value: "AWS, GCP, Azure"}},
			wantApplied:  0,
			wantRejected: 1,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if len(result.EditsRejected) == 0 {
					t.Fatal("expected rejection for missing category, got none")
				}
				reason := result.EditsRejected[0].Reason
				if !strings.Contains(reason, "requires a category") {
					t.Errorf("rejection reason %q must contain %q", reason, "requires a category")
				}
				if !strings.Contains(reason, "available:") {
					t.Errorf("rejection reason %q must contain %q", reason, "available:")
				}
				cats := result.NewSections.Skills.Categorized
				if items, ok := cats["Cloud"]; !ok || len(items) != 2 {
					t.Errorf("Categorized map was mutated: Cloud = %v", cats["Cloud"])
				}
				if result.NewSections.Skills.Flat != "" {
					t.Errorf("Skills.Flat must remain empty for Categorized kind, got %q", result.NewSections.Skills.Flat)
				}
			},
		},
		{
			name: "skills categorized rejects ops with unknown category",
			sections: func() model.SectionMap {
				return model.SectionMap{
					Skills: &model.SkillsSection{
						Kind:        model.SkillsKindCategorized,
						Categorized: map[string][]string{"Cloud": {"AWS", "GCP"}},
					},
				}
			},
			edits:        []port.Edit{{Section: "skills", Op: port.EditOpAdd, Value: "Spark", Category: "Nonexistent"}},
			wantApplied:  0,
			wantRejected: 1,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if len(result.EditsRejected) == 0 {
					t.Fatal("expected rejection for unknown category, got none")
				}
				reason := result.EditsRejected[0].Reason
				if !strings.Contains(reason, `category "Nonexistent" not found`) {
					t.Errorf("rejection reason %q must contain %q", reason, `category "Nonexistent" not found`)
				}
				if !strings.Contains(reason, "available:") {
					t.Errorf("rejection reason %q must contain %q", reason, "available:")
				}
				cats := result.NewSections.Skills.Categorized
				if items, ok := cats["Cloud"]; !ok || len(items) != 2 {
					t.Errorf("Categorized map was mutated: Cloud = %v", cats["Cloud"])
				}
			},
		},
		{
			name: "skills categorized add appends items to named category",
			sections: func() model.SectionMap {
				return model.SectionMap{
					Skills: &model.SkillsSection{
						Kind:        model.SkillsKindCategorized,
						Categorized: map[string][]string{"Cloud": {"AWS", "GCP"}},
					},
				}
			},
			edits:        []port.Edit{{Section: "skills", Op: port.EditOpAdd, Value: "Apache Kafka, Spark", Category: "Cloud"}},
			wantApplied:  1,
			wantRejected: 0,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				cloud := result.NewSections.Skills.Categorized["Cloud"]
				if len(cloud) != 4 {
					t.Errorf("expected 4 items in Cloud after add, got %d: %v", len(cloud), cloud)
				}
				found := map[string]bool{}
				for _, item := range cloud {
					found[item] = true
				}
				if !found["Apache Kafka"] {
					t.Errorf("Cloud does not contain %q after add: %v", "Apache Kafka", cloud)
				}
				if !found["Spark"] {
					t.Errorf("Cloud does not contain %q after add: %v", "Spark", cloud)
				}
				if result.NewSections.Skills.Kind != model.SkillsKindCategorized {
					t.Errorf("Kind = %q, want %q", result.NewSections.Skills.Kind, model.SkillsKindCategorized)
				}
				if result.NewSections.Skills.Flat != "" {
					t.Errorf("Skills.Flat must remain empty, got %q", result.NewSections.Skills.Flat)
				}
			},
		},
		{
			name: "skills categorized replace sets named category",
			sections: func() model.SectionMap {
				return model.SectionMap{
					Skills: &model.SkillsSection{
						Kind:        model.SkillsKindCategorized,
						Categorized: map[string][]string{"Cloud": {"AWS", "GCP"}},
					},
				}
			},
			edits:        []port.Edit{{Section: "skills", Op: port.EditOpReplace, Value: "Azure, GCP", Category: "Cloud"}},
			wantApplied:  1,
			wantRejected: 0,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				cloud := result.NewSections.Skills.Categorized["Cloud"]
				if len(cloud) != 2 {
					t.Errorf("expected 2 items in Cloud after replace, got %d: %v", len(cloud), cloud)
				}
				if cloud[0] != "Azure" || cloud[1] != "GCP" {
					t.Errorf("Cloud = %v, want [Azure GCP]", cloud)
				}
				if result.NewSections.Skills.Kind != model.SkillsKindCategorized {
					t.Errorf("Kind = %q, want %q", result.NewSections.Skills.Kind, model.SkillsKindCategorized)
				}
				if result.NewSections.Skills.Flat != "" {
					t.Errorf("Skills.Flat must remain empty, got %q", result.NewSections.Skills.Flat)
				}
			},
		},
		{
			name: "skills categorized add with comma separated value splits items",
			sections: func() model.SectionMap {
				return model.SectionMap{
					Skills: &model.SkillsSection{
						Kind:        model.SkillsKindCategorized,
						Categorized: map[string][]string{"Cloud": {}},
					},
				}
			},
			edits:        []port.Edit{{Section: "skills", Op: port.EditOpAdd, Value: "AWS, GCP", Category: "Cloud"}},
			wantApplied:  1,
			wantRejected: 0,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				cloud := result.NewSections.Skills.Categorized["Cloud"]
				if len(cloud) != 2 {
					t.Errorf("expected 2 separate items, got %d: %v", len(cloud), cloud)
				}
				if len(cloud) == 1 {
					t.Errorf("value was not split — got single entry %q instead of two", cloud[0])
				}
			},
		},
		{
			name: "skills categorized empty map rejection lists no categories",
			sections: func() model.SectionMap {
				return model.SectionMap{
					Skills: &model.SkillsSection{
						Kind:        model.SkillsKindCategorized,
						Categorized: map[string][]string{},
					},
				}
			},
			edits:        []port.Edit{{Section: "skills", Op: port.EditOpAdd, Value: "Go"}},
			wantApplied:  0,
			wantRejected: 1,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if len(result.EditsRejected) == 0 {
					t.Fatal("expected rejection for empty categorized map, got none")
				}
				reason := result.EditsRejected[0].Reason
				if !strings.Contains(reason, "available:") {
					t.Errorf("rejection reason %q must contain %q", reason, "available:")
				}
			},
		},
		{
			name: "skills flat ignores category field",
			sections: func() model.SectionMap {
				return model.SectionMap{
					Skills: &model.SkillsSection{
						Kind: model.SkillsKindFlat,
						Flat: "Go, Python",
					},
				}
			},
			edits:        []port.Edit{{Section: "skills", Op: port.EditOpAdd, Value: "Rust", Category: "SomeCategory"}},
			wantApplied:  1,
			wantRejected: 0,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				if !strings.Contains(result.NewSections.Skills.Flat, "Rust") {
					t.Errorf("Skills.Flat %q does not contain %q after add with category on flat section", result.NewSections.Skills.Flat, "Rust")
				}
			},
		},
		{
			name: "skills categorized mixed call applies valid and rejects invalid",
			sections: func() model.SectionMap {
				return model.SectionMap{
					Skills: &model.SkillsSection{
						Kind:        model.SkillsKindCategorized,
						Categorized: map[string][]string{"Cloud": {"AWS"}, "Languages": {"Go"}},
					},
				}
			},
			edits: []port.Edit{
				{Section: "skills", Op: port.EditOpAdd, Value: "Azure", Category: "Cloud"},
				{Section: "skills", Op: port.EditOpAdd, Value: "Spark", Category: "Nonexistent"},
			},
			wantApplied:  1,
			wantRejected: 1,
			check: func(t *testing.T, result port.EditResult) {
				t.Helper()
				cloud := result.NewSections.Skills.Categorized["Cloud"]
				found := false
				for _, item := range cloud {
					if item == "Azure" {
						found = true
					}
				}
				if !found {
					t.Errorf("Cloud should contain Azure after valid edit: %v", cloud)
				}
				if len(result.EditsRejected) == 0 {
					t.Fatal("expected rejection for unknown category, got none")
				}
				reason := result.EditsRejected[0].Reason
				if !strings.Contains(reason, "not found") {
					t.Errorf("rejection reason %q must contain %q", reason, "not found")
				}
				if !strings.Contains(reason, "available:") {
					t.Errorf("rejection reason %q must contain %q", reason, "available:")
				}
			},
		},
	}
	for _, ct := range catTests {
		tests = append(tests, struct {
			name         string
			sections     func() model.SectionMap
			edits        []port.Edit
			wantApplied  int
			wantRejected int
			check        func(t *testing.T, result port.EditResult)
		}(ct))
	}

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
