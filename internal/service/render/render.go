package render

import (
	"sort"
	"strings"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

var _ port.Renderer = (*Service)(nil)

// Service renders a SectionMap to ATS-safe plain text in canonical section order.
type Service struct{}

func New() *Service { return &Service{} }

// sectionWriter holds the write function for one resume section.
// Ordered slice index determines render order; SectionMap.Order is validated but not consulted here.
type sectionWriter struct {
	write func(b *strings.Builder, s *model.SectionMap)
}

// sectionRegistry defines canonical render order. Append new sections here.
var sectionRegistry = []sectionWriter{
	{func(b *strings.Builder, s *model.SectionMap) { writeContact(b, &s.Contact) }},
	{func(b *strings.Builder, s *model.SectionMap) {
		writeSection(b, "SUMMARY", func() { b.WriteString(s.Summary + "\n") }, s.Summary != "")
	}},
	{func(b *strings.Builder, s *model.SectionMap) { writeExperience(b, s.Experience) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeEducation(b, s.Education) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeSkills(b, s.Skills) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeProjects(b, s.Projects) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeCertifications(b, s.Certifications) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeAwards(b, s.Awards) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeVolunteer(b, s.Volunteer) }},
	{func(b *strings.Builder, s *model.SectionMap) { writePublications(b, s.Publications) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeLanguages(b, s.Languages) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeSpeaking(b, s.Speaking) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeOpenSource(b, s.OpenSource) }},
	{func(b *strings.Builder, s *model.SectionMap) { writePatents(b, s.Patents) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeInterests(b, s.Interests) }},
	{func(b *strings.Builder, s *model.SectionMap) { writeReferences(b, s.References) }},
}

// Render converts sections to plain text. Sections are emitted in canonical order;
// absent/empty sections are skipped. Returns empty string for nil input.
func (s *Service) Render(sections *model.SectionMap) (string, error) {
	if sections == nil {
		return "", nil
	}
	var sb strings.Builder
	for _, w := range sectionRegistry {
		w.write(&sb, sections)
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

func writeContact(sb *strings.Builder, c *model.ContactInfo) {
	if c.Name == "" {
		return
	}
	sb.WriteString(c.Name + "\n")
	if c.Email != "" {
		sb.WriteString(c.Email + "\n")
	}
	if c.Phone != "" {
		sb.WriteString(c.Phone + "\n")
	}
	if c.Location != "" {
		sb.WriteString(c.Location + "\n")
	}
	for _, link := range c.Links {
		sb.WriteString(link + "\n")
	}
	sb.WriteString("\n")
}

func writeSection(sb *strings.Builder, heading string, body func(), condition bool) {
	if !condition {
		return
	}
	sb.WriteString(heading + "\n")
	body()
	sb.WriteString("\n")
}

func writeExperience(sb *strings.Builder, entries []model.ExperienceEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("EXPERIENCE\n")
	for _, e := range entries {
		line := e.Company + " | " + e.Role + " | " + e.StartDate
		if e.EndDate != "" {
			line += " - " + e.EndDate
		}
		sb.WriteString(line + "\n")
		for _, b := range e.Bullets {
			sb.WriteString("- " + b + "\n")
		}
		sb.WriteString("\n")
	}
}

func writeEducation(sb *strings.Builder, entries []model.EducationEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("EDUCATION\n")
	for _, e := range entries {
		sb.WriteString(e.School + " | " + e.Degree + "\n")
		if e.Details != "" {
			sb.WriteString(e.Details + "\n")
		}
	}
	sb.WriteString("\n")
}

func writeSkills(sb *strings.Builder, skills *model.SkillsSection) {
	if skills == nil {
		return
	}
	sb.WriteString("SKILLS\n")
	if skills.Flat != "" {
		sb.WriteString(skills.Flat + "\n")
	} else {
		// Emit categories in sorted order for determinism.
		cats := make([]string, 0, len(skills.Categorized))
		for c := range skills.Categorized {
			cats = append(cats, c)
		}
		sort.Strings(cats)
		for _, cat := range cats {
			sb.WriteString(cat + ": " + strings.Join(skills.Categorized[cat], ", ") + "\n")
		}
	}
	sb.WriteString("\n")
}

func writeProjects(sb *strings.Builder, entries []model.ProjectEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("PROJECTS\n")
	for _, p := range entries {
		sb.WriteString(p.Name + "\n")
		if p.Description != "" {
			sb.WriteString(p.Description + "\n")
		}
		for _, b := range p.Bullets {
			sb.WriteString("- " + b + "\n")
		}
	}
	sb.WriteString("\n")
}

func writeCertifications(sb *strings.Builder, entries []model.CertificationEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("CERTIFICATIONS\n")
	for _, c := range entries {
		sb.WriteString(c.Name + "\n")
	}
	sb.WriteString("\n")
}

func writeAwards(sb *strings.Builder, entries []model.AwardEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("AWARDS\n")
	for _, a := range entries {
		sb.WriteString(a.Title + "\n")
	}
	sb.WriteString("\n")
}

func writeVolunteer(sb *strings.Builder, entries []model.VolunteerEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("VOLUNTEER\n")
	for _, v := range entries {
		sb.WriteString(v.Org + " | " + v.Role + "\n")
		for _, b := range v.Bullets {
			sb.WriteString("- " + b + "\n")
		}
	}
	sb.WriteString("\n")
}

func writePublications(sb *strings.Builder, entries []model.PublicationEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("PUBLICATIONS\n")
	for _, p := range entries {
		sb.WriteString(p.Title + "\n")
	}
	sb.WriteString("\n")
}

// Tier 4 writers intentionally emit a minimal plain-text representation (name/title only).
// Omitted fields (URL, Contact, Date, etc.) are preserved in the sections file
// and are available for future renderers — this is not a silent failure.

func writeLanguages(sb *strings.Builder, entries []model.LanguageEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("LANGUAGES\n")
	for _, e := range entries {
		if e.Proficiency != "" {
			sb.WriteString(e.Name + ": " + e.Proficiency + "\n")
		} else {
			sb.WriteString(e.Name + "\n")
		}
	}
	sb.WriteString("\n")
}

func writeSpeaking(sb *strings.Builder, entries []model.SpeakingEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("SPEAKING ENGAGEMENTS\n")
	for _, e := range entries {
		sb.WriteString(e.Title + "\n")
		if e.Event != "" {
			sb.WriteString(e.Event + "\n")
		}
	}
	sb.WriteString("\n")
}

func writeOpenSource(sb *strings.Builder, entries []model.OpenSourceEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("OPEN SOURCE\n")
	for _, e := range entries {
		sb.WriteString(e.Project + "\n")
		if e.Description != "" {
			sb.WriteString(e.Description + "\n")
		}
	}
	sb.WriteString("\n")
}

func writePatents(sb *strings.Builder, entries []model.PatentEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("PATENTS\n")
	for _, e := range entries {
		sb.WriteString(e.Title + "\n")
	}
	sb.WriteString("\n")
}

func writeInterests(sb *strings.Builder, entries []model.InterestEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("INTERESTS\n")
	for _, e := range entries {
		sb.WriteString(e.Name + "\n")
	}
	sb.WriteString("\n")
}

func writeReferences(sb *strings.Builder, entries []model.ReferenceEntry) {
	if len(entries) == 0 {
		return
	}
	sb.WriteString("REFERENCES\n")
	for _, e := range entries {
		sb.WriteString(e.Name + "\n")
	}
	sb.WriteString("\n")
}
