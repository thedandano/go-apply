package render_test

import (
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/render"
)

var _ port.Renderer = render.New()

func TestRender_NilSections_ReturnsEmpty(t *testing.T) {
	svc := render.New()
	out, err := svc.Render(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty string for nil sections, got: %q", out)
	}
}

func TestRender_ContactOnly(t *testing.T) {
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact: model.ContactInfo{
			Name:     "Jane Doe",
			Email:    "jane@example.com",
			Phone:    "555-1234",
			Location: "San Francisco, CA",
			Links:    []string{"linkedin.com/in/jane"},
		},
	}
	svc := render.New()
	out, err := svc.Render(sm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Jane Doe", "jane@example.com", "555-1234", "San Francisco, CA", "linkedin.com/in/jane"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRender_Experience_ContainsBullets(t *testing.T) {
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Test"},
		Experience: []model.ExperienceEntry{
			{
				Company:   "Acme Corp",
				Role:      "Senior Engineer",
				StartDate: "2020-01",
				EndDate:   "2023-12",
				Bullets:   []string{"Led team of 10", "Reduced latency by 40%"},
			},
		},
	}
	svc := render.New()
	out, err := svc.Render(sm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Acme Corp", "Senior Engineer", "2020-01", "2023-12", "Led team of 10", "Reduced latency by 40%"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRender_SkillsFlat(t *testing.T) {
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Test"},
		Skills: &model.SkillsSection{
			Kind: model.SkillsKindFlat,
			Flat: "Go, Python, Kubernetes",
		},
	}
	svc := render.New()
	out, err := svc.Render(sm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Go, Python, Kubernetes") {
		t.Errorf("output missing skills flat content; got:\n%s", out)
	}
}

func TestRender_SkillsCategorized(t *testing.T) {
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Test"},
		Skills: &model.SkillsSection{
			Kind:        model.SkillsKindCategorized,
			Categorized: map[string][]string{"Cloud": {"AWS", "GCP"}, "Languages": {"Go"}},
		},
	}
	svc := render.New()
	out, err := svc.Render(sm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"AWS", "GCP", "Go"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q from categorized skills; got:\n%s", want, out)
		}
	}
}

func TestRender_AllSections_ContainsKeywords(t *testing.T) {
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Alice"},
		Summary:       "Experienced platform engineer.",
		Experience: []model.ExperienceEntry{
			{Company: "BigCo", Role: "Staff Eng", StartDate: "2021-01", Bullets: []string{"Built pipelines"}},
		},
		Education: []model.EducationEntry{
			{School: "MIT", Degree: "BSc CS"},
		},
		Skills:         &model.SkillsSection{Kind: model.SkillsKindFlat, Flat: "Go, Terraform"},
		Projects:       []model.ProjectEntry{{Name: "Open Source Tool", Bullets: []string{"Released on GitHub"}}},
		Certifications: []model.CertificationEntry{{Name: "AWS Certified"}},
		Awards:         []model.AwardEntry{{Title: "Hackathon Winner"}},
		Volunteer:      []model.VolunteerEntry{{Org: "Code for Good", Role: "Mentor", Bullets: []string{"Taught Python"}}},
		Publications:   []model.PublicationEntry{{Title: "Distributed Systems Paper"}},
	}
	svc := render.New()
	out, err := svc.Render(sm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"Alice", "Experienced platform engineer", "BigCo", "Built pipelines",
		"MIT", "Go, Terraform", "Open Source Tool", "AWS Certified",
		"Hackathon Winner", "Code for Good", "Taught Python", "Distributed Systems Paper",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}

// T009: Each non-empty Tier 4 section must produce its canonical heading.
// These tests must FAIL until T011/T012 add the Tier 4 writers.
func TestRender_Tier4Headings_AppearWhenNonEmpty(t *testing.T) {
	base := model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Test"},
		Experience:    []model.ExperienceEntry{{Company: "Acme", Role: "Eng", StartDate: "2020-01", Bullets: []string{}}},
	}
	cases := []struct {
		name    string
		mutate  func(*model.SectionMap)
		heading string
	}{
		{"Languages", func(sm *model.SectionMap) {
			sm.Languages = []model.LanguageEntry{{Name: "Go", Proficiency: "Fluent"}}
		}, "LANGUAGES"},
		{"Speaking", func(sm *model.SectionMap) {
			sm.Speaking = []model.SpeakingEntry{{Title: "GopherCon", Event: "Conf", Date: "2023"}}
		}, "SPEAKING ENGAGEMENTS"},
		{"OpenSource", func(sm *model.SectionMap) {
			sm.OpenSource = []model.OpenSourceEntry{{Project: "go-apply", Role: "Author"}}
		}, "OPEN SOURCE"},
		{"Patents", func(sm *model.SectionMap) {
			sm.Patents = []model.PatentEntry{{Title: "Algorithm", Number: "US123"}}
		}, "PATENTS"},
		{"Interests", func(sm *model.SectionMap) {
			sm.Interests = []model.InterestEntry{{Name: "Open Source"}}
		}, "INTERESTS"},
		{"References", func(sm *model.SectionMap) {
			sm.References = []model.ReferenceEntry{{Name: "Available upon request"}}
		}, "REFERENCES"},
	}

	svc := render.New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sm := base
			tc.mutate(&sm)
			out, err := svc.Render(&sm)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			if !strings.Contains(out, tc.heading+"\n") {
				t.Errorf("expected heading %q in output; got:\n%s", tc.heading, out)
			}
		})
	}
}

// Edge cases for Tier 4 writer branches.
func TestRender_Tier4WriterEdgeCases(t *testing.T) {
	svc := render.New()

	t.Run("Language_without_proficiency_renders_name_only", func(t *testing.T) {
		sm := &model.SectionMap{
			SchemaVersion: model.CurrentSchemaVersion,
			Contact:       model.ContactInfo{Name: "Test"},
			Languages:     []model.LanguageEntry{{Name: "Rust"}}, // Proficiency empty
		}
		out, err := svc.Render(sm)
		if err != nil {
			t.Fatalf("Render error: %v", err)
		}
		if !strings.Contains(out, "Rust") {
			t.Errorf("expected bare name 'Rust'; got:\n%s", out)
		}
		if strings.Contains(out, "Rust:") {
			t.Errorf("must not emit colon-separator when Proficiency is empty; got:\n%s", out)
		}
	})

	t.Run("Speaking_without_event_skips_event_line", func(t *testing.T) {
		sm := &model.SectionMap{
			SchemaVersion: model.CurrentSchemaVersion,
			Contact:       model.ContactInfo{Name: "Test"},
			Speaking:      []model.SpeakingEntry{{Title: "My Talk", Event: ""}}, // Event empty
		}
		out, err := svc.Render(sm)
		if err != nil {
			t.Fatalf("Render error: %v", err)
		}
		if !strings.Contains(out, "My Talk") {
			t.Errorf("expected title 'My Talk'; got:\n%s", out)
		}
		// The section should have exactly 2 non-empty lines after SPEAKING ENGAGEMENTS heading:
		// heading + title only (no event line).
		idx := strings.Index(out, "SPEAKING ENGAGEMENTS\n")
		if idx < 0 {
			t.Fatalf("SPEAKING ENGAGEMENTS heading not found")
		}
		section := out[idx:]
		if end := strings.Index(section[len("SPEAKING ENGAGEMENTS\n"):], "\n\n"); end >= 0 {
			section = section[:len("SPEAKING ENGAGEMENTS\n")+end]
		}
		nonEmpty := 0
		for _, l := range strings.Split(section, "\n") {
			if strings.TrimSpace(l) != "" {
				nonEmpty++
			}
		}
		if nonEmpty != 2 { // heading + title, no event
			t.Errorf("expected 2 non-empty lines (heading + title) in Speaking section without Event; got %d in:\n%s", nonEmpty, section)
		}
	})
}

// T010: Empty Tier 4 slices must produce no heading in the output.
// This test passes before AND after T011/T012 (empty = no-op is the expected behaviour).
func TestRender_Tier4Headings_AbsentWhenEmpty(t *testing.T) {
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Test"},
		Experience:    []model.ExperienceEntry{{Company: "Acme", Role: "Eng", StartDate: "2020-01", Bullets: []string{}}},
		Languages:     nil,
		Speaking:      nil,
		OpenSource:    nil,
		Patents:       nil,
		Interests:     nil,
		References:    nil,
	}
	svc := render.New()
	out, err := svc.Render(sm)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	for _, heading := range []string{"LANGUAGES", "SPEAKING ENGAGEMENTS", "OPEN SOURCE", "PATENTS", "INTERESTS", "REFERENCES"} {
		if strings.Contains(out, heading) {
			t.Errorf("empty Tier 4 slice must not produce heading %q; got:\n%s", heading, out)
		}
	}
}

func TestRender_Deterministic(t *testing.T) {
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Bob"},
		Skills:        &model.SkillsSection{Kind: model.SkillsKindFlat, Flat: "Go"},
	}
	svc := render.New()
	a, _ := svc.Render(sm)
	b, _ := svc.Render(sm)
	if a != b {
		t.Error("Render must be deterministic — two calls with same input returned different output")
	}
}
