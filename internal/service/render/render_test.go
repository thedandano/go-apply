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
