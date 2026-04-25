package render

// T008: Dual-render regression test (SC-006).
// This helper replicates the pre-refactor hardcoded dispatch so the test can
// assert that the registry-based Render produces byte-for-byte identical output.
// Kept alongside the implementation; remove in a follow-up once the registry is
// the sole render path.

import (
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
)

func preRefactorRender(sections *model.SectionMap) string {
	if sections == nil {
		return ""
	}
	var sb strings.Builder
	writeContact(&sb, &sections.Contact)
	writeSection(&sb, "SUMMARY", func() { sb.WriteString(sections.Summary + "\n") }, sections.Summary != "")
	writeExperience(&sb, sections.Experience)
	writeEducation(&sb, sections.Education)
	writeSkills(&sb, sections.Skills)
	writeProjects(&sb, sections.Projects)
	writeCertifications(&sb, sections.Certifications)
	writeAwards(&sb, sections.Awards)
	writeVolunteer(&sb, sections.Volunteer)
	writePublications(&sb, sections.Publications)
	return strings.TrimRight(sb.String(), "\n")
}

func TestRender_SC006_DualRender(t *testing.T) {
	fixture := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact: model.ContactInfo{
			Name:     "Alice",
			Email:    "alice@example.com",
			Phone:    "555-0100",
			Location: "New York, NY",
			Links:    []string{"github.com/alice"},
		},
		Summary: "Platform engineer with 8 years of experience.",
		Experience: []model.ExperienceEntry{
			{Company: "BigCo", Role: "Staff Eng", StartDate: "2021-01", EndDate: "2024-12", Bullets: []string{"Built pipelines", "Led migration"}},
		},
		Education:      []model.EducationEntry{{School: "MIT", Degree: "BSc CS"}},
		Skills:         &model.SkillsSection{Kind: model.SkillsKindFlat, Flat: "Go, Terraform"},
		Projects:       []model.ProjectEntry{{Name: "go-apply", Bullets: []string{"Open source CLI"}}},
		Certifications: []model.CertificationEntry{{Name: "AWS Certified"}},
		Awards:         []model.AwardEntry{{Title: "Hackathon Winner"}},
		Volunteer:      []model.VolunteerEntry{{Org: "Code for Good", Role: "Mentor", Bullets: []string{"Taught Python"}}},
		Publications:   []model.PublicationEntry{{Title: "Distributed Systems Paper"}},
	}

	svc := New()
	got, err := svc.Render(fixture)
	if err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	want := preRefactorRender(fixture)
	if got != want {
		t.Errorf("registry Render differs from pre-refactor output\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}
