package pdfrender

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"unicode/utf8"

	"github.com/go-pdf/fpdf"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Service implements port.PDFRenderer.
type Service struct{}

var _ port.PDFRenderer = (*Service)(nil)

// New returns a new PDF renderer service.
func New() *Service { return &Service{} }

// RenderPDF converts a SectionMap to ATS-safe PDF bytes.
func (s *Service) RenderPDF(sections *model.SectionMap) ([]byte, error) {
	if sections == nil {
		return nil, errors.New("pdfrender: nil sections")
	}

	if err := validateUTF8Fields(sections); err != nil {
		return nil, fmt.Errorf("pdfrender: invalid UTF-8 in field: %w", err)
	}

	transliterated := transliterateLatin1(sections)
	sections = &transliterated

	slog.Info("pdfrender.render", "sections_count", countNonEmptySections(sections))

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 20, 15)
	pdf.AddPage()
	pdf.SetFont("Arial", "", 10)

	writeContactPDF(pdf, &sections.Contact)
	writeSummaryPDF(pdf, sections.Summary)
	writeExperiencePDF(pdf, sections.Experience)
	writeEducationPDF(pdf, sections.Education)
	writeSkillsPDF(pdf, sections.Skills)
	writeProjectsPDF(pdf, sections.Projects)
	writeCertificationsPDF(pdf, sections.Certifications)
	writeAwardsPDF(pdf, sections.Awards)
	writeVolunteerPDF(pdf, sections.Volunteer)
	writePublicationsPDF(pdf, sections.Publications)
	writeLanguagesPDF(pdf, sections.Languages)
	writeSpeakingPDF(pdf, sections.Speaking)
	writeOpenSourcePDF(pdf, sections.OpenSource)
	writePatentsPDF(pdf, sections.Patents)
	writeInterestsPDF(pdf, sections.Interests)
	writeReferencesPDF(pdf, sections.References)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("pdfrender: output failed: %w", err)
	}

	// Check fpdf's internal error state (accumulated during section writes).
	if err := pdf.Error(); err != nil {
		return nil, fmt.Errorf("pdfrender: internal rendering error: %w", err)
	}

	pdfBytes := buf.Bytes()
	if len(pdfBytes) == 0 {
		return nil, errors.New("pdfrender: zero-length output")
	}

	slog.Info("pdfrender.done", "bytes", len(pdfBytes))
	return pdfBytes, nil
}

// validateUTF8Fields checks all string fields in sections for valid UTF-8.
func validateUTF8Fields(sections *model.SectionMap) error {
	check := func(field, value string) error {
		if !utf8.ValidString(value) {
			return fmt.Errorf("field %q contains invalid UTF-8", field)
		}
		return nil
	}

	// Contact
	if err := check("contact.name", sections.Contact.Name); err != nil {
		return err
	}
	if err := check("contact.email", sections.Contact.Email); err != nil {
		return err
	}
	if err := check("contact.phone", sections.Contact.Phone); err != nil {
		return err
	}
	if err := check("contact.location", sections.Contact.Location); err != nil {
		return err
	}
	for i, link := range sections.Contact.Links {
		if err := check(fmt.Sprintf("contact.links[%d]", i), link); err != nil {
			return err
		}
	}

	// Summary
	if err := check("summary", sections.Summary); err != nil {
		return err
	}

	// Experience
	for i, e := range sections.Experience {
		prefix := fmt.Sprintf("experience[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".company", e.Company},
			{prefix + ".role", e.Role},
			{prefix + ".start_date", e.StartDate},
			{prefix + ".end_date", e.EndDate},
			{prefix + ".location", e.Location},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
		for j, b := range e.Bullets {
			if err := check(fmt.Sprintf("%s.bullets[%d]", prefix, j), b); err != nil {
				return err
			}
		}
	}

	// Education
	for i, e := range sections.Education {
		prefix := fmt.Sprintf("education[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".school", e.School},
			{prefix + ".degree", e.Degree},
			{prefix + ".start_date", e.StartDate},
			{prefix + ".end_date", e.EndDate},
			{prefix + ".location", e.Location},
			{prefix + ".details", e.Details},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
	}

	// Skills
	if sections.Skills != nil {
		if err := check("skills.flat", sections.Skills.Flat); err != nil {
			return err
		}
		for cat, vals := range sections.Skills.Categorized {
			if err := check("skills.categorized.key:"+cat, cat); err != nil {
				return err
			}
			for j, v := range vals {
				if err := check(fmt.Sprintf("skills.categorized[%s][%d]", cat, j), v); err != nil {
					return err
				}
			}
		}
	}

	// Projects
	for i, p := range sections.Projects {
		prefix := fmt.Sprintf("projects[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".name", p.Name},
			{prefix + ".description", p.Description},
			{prefix + ".url", p.URL},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
		for j, b := range p.Bullets {
			if err := check(fmt.Sprintf("%s.bullets[%d]", prefix, j), b); err != nil {
				return err
			}
		}
	}

	// Certifications
	for i, c := range sections.Certifications {
		prefix := fmt.Sprintf("certifications[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".name", c.Name},
			{prefix + ".issuer", c.Issuer},
			{prefix + ".date", c.Date},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
	}

	// Awards
	for i, a := range sections.Awards {
		prefix := fmt.Sprintf("awards[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".title", a.Title},
			{prefix + ".date", a.Date},
			{prefix + ".details", a.Details},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
	}

	// Volunteer
	for i, v := range sections.Volunteer {
		prefix := fmt.Sprintf("volunteer[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".org", v.Org},
			{prefix + ".role", v.Role},
			{prefix + ".start_date", v.StartDate},
			{prefix + ".end_date", v.EndDate},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
		for j, b := range v.Bullets {
			if err := check(fmt.Sprintf("%s.bullets[%d]", prefix, j), b); err != nil {
				return err
			}
		}
	}

	// Publications
	for i, p := range sections.Publications {
		prefix := fmt.Sprintf("publications[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".title", p.Title},
			{prefix + ".venue", p.Venue},
			{prefix + ".date", p.Date},
			{prefix + ".url", p.URL},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
	}

	// Languages
	for i, e := range sections.Languages {
		prefix := fmt.Sprintf("languages[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".name", e.Name},
			{prefix + ".proficiency", e.Proficiency},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
	}

	// Speaking
	for i, e := range sections.Speaking {
		prefix := fmt.Sprintf("speaking[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".title", e.Title},
			{prefix + ".event", e.Event},
			{prefix + ".date", e.Date},
			{prefix + ".url", e.URL},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
	}

	// OpenSource
	for i, e := range sections.OpenSource {
		prefix := fmt.Sprintf("open_source[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".project", e.Project},
			{prefix + ".role", e.Role},
			{prefix + ".url", e.URL},
			{prefix + ".description", e.Description},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
	}

	// Patents
	for i, e := range sections.Patents {
		prefix := fmt.Sprintf("patents[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".title", e.Title},
			{prefix + ".number", e.Number},
			{prefix + ".date", e.Date},
			{prefix + ".url", e.URL},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
	}

	// Interests
	for i, e := range sections.Interests {
		if err := check(fmt.Sprintf("interests[%d].name", i), e.Name); err != nil {
			return err
		}
	}

	// References
	for i, e := range sections.References {
		prefix := fmt.Sprintf("references[%d]", i)
		for _, f := range []struct{ name, val string }{
			{prefix + ".name", e.Name},
			{prefix + ".title", e.Title},
			{prefix + ".company", e.Company},
			{prefix + ".contact", e.Contact},
		} {
			if err := check(f.name, f.val); err != nil {
				return err
			}
		}
	}

	return nil
}

// countNonEmptySections returns the number of non-empty sections in the SectionMap.
func countNonEmptySections(sections *model.SectionMap) int {
	count := 0
	if sections.Contact.Name != "" {
		count++
	}
	if sections.Summary != "" {
		count++
	}
	if len(sections.Experience) > 0 {
		count++
	}
	if len(sections.Education) > 0 {
		count++
	}
	if sections.Skills != nil {
		count++
	}
	if len(sections.Projects) > 0 {
		count++
	}
	if len(sections.Certifications) > 0 {
		count++
	}
	if len(sections.Awards) > 0 {
		count++
	}
	if len(sections.Volunteer) > 0 {
		count++
	}
	if len(sections.Publications) > 0 {
		count++
	}
	if len(sections.Languages) > 0 {
		count++
	}
	if len(sections.Speaking) > 0 {
		count++
	}
	if len(sections.OpenSource) > 0 {
		count++
	}
	if len(sections.Patents) > 0 {
		count++
	}
	if len(sections.Interests) > 0 {
		count++
	}
	if len(sections.References) > 0 {
		count++
	}
	return count
}

// sectionHeading writes a bold section heading line.
func sectionHeading(pdf *fpdf.Fpdf, title string) {
	pdf.SetFont("Arial", "B", 11)
	pdf.MultiCell(0, 6, title, "", "L", false)
	pdf.SetFont("Arial", "", 10)
}

// writeContactPDF renders the contact section.
func writeContactPDF(pdf *fpdf.Fpdf, c *model.ContactInfo) {
	if c.Name == "" {
		return
	}
	pdf.SetFont("Arial", "B", 14)
	pdf.MultiCell(0, 8, c.Name, "", "C", false)
	pdf.SetFont("Arial", "", 10)

	var parts []string
	if c.Email != "" {
		parts = append(parts, c.Email)
	}
	if c.Phone != "" {
		parts = append(parts, c.Phone)
	}
	if c.Location != "" {
		parts = append(parts, c.Location)
	}
	for _, link := range c.Links {
		if link != "" {
			parts = append(parts, link)
		}
	}

	if len(parts) > 0 {
		line := ""
		for i, p := range parts {
			if i > 0 {
				line += " | "
			}
			line += p
		}
		pdf.MultiCell(0, 6, line, "", "C", false)
	}
	pdf.Ln(4)
}

// writeSummaryPDF renders the summary section.
func writeSummaryPDF(pdf *fpdf.Fpdf, summary string) {
	if summary == "" {
		return
	}
	sectionHeading(pdf, "SUMMARY")
	pdf.MultiCell(0, 6, summary, "", "L", false)
	pdf.Ln(4)
}

// writeExperiencePDF renders the experience section.
func writeExperiencePDF(pdf *fpdf.Fpdf, entries []model.ExperienceEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "EXPERIENCE")
	for _, e := range entries {
		pdf.SetFont("Arial", "B", 10)
		line := e.Company + " | " + e.Role + " | " + e.StartDate
		if e.EndDate != "" {
			line += " - " + e.EndDate
		}
		if e.Location != "" {
			line += " (" + e.Location + ")"
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
		pdf.SetFont("Arial", "", 10)
		for _, b := range e.Bullets {
			pdf.MultiCell(0, 6, "- "+b, "", "L", false)
		}
		pdf.Ln(2)
	}
	pdf.Ln(2)
}

// writeEducationPDF renders the education section.
func writeEducationPDF(pdf *fpdf.Fpdf, entries []model.EducationEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "EDUCATION")
	for _, e := range entries {
		pdf.SetFont("Arial", "B", 10)
		line := e.School + " | " + e.Degree
		if e.EndDate != "" {
			line += " | " + e.EndDate
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
		pdf.SetFont("Arial", "", 10)
		if e.Details != "" {
			pdf.MultiCell(0, 6, e.Details, "", "L", false)
		}
		pdf.Ln(2)
	}
	pdf.Ln(2)
}

// writeSkillsPDF renders the skills section.
func writeSkillsPDF(pdf *fpdf.Fpdf, skills *model.SkillsSection) {
	if skills == nil {
		return
	}
	sectionHeading(pdf, "SKILLS")
	pdf.SetFont("Arial", "", 10)
	if skills.Flat != "" {
		pdf.MultiCell(0, 6, skills.Flat, "", "L", false)
	} else {
		cats := make([]string, 0, len(skills.Categorized))
		for c := range skills.Categorized {
			cats = append(cats, c)
		}
		sort.Strings(cats)
		for _, cat := range cats {
			vals := skills.Categorized[cat]
			pdf.MultiCell(0, 6, cat+": "+join(vals, ", "), "", "L", false)
		}
	}
	pdf.Ln(4)
}

// writeProjectsPDF renders the projects section.
func writeProjectsPDF(pdf *fpdf.Fpdf, entries []model.ProjectEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "PROJECTS")
	for _, p := range entries {
		pdf.SetFont("Arial", "B", 10)
		name := p.Name
		if p.URL != "" {
			name += " (" + p.URL + ")"
		}
		pdf.MultiCell(0, 6, name, "", "L", false)
		pdf.SetFont("Arial", "", 10)
		if p.Description != "" {
			pdf.MultiCell(0, 6, p.Description, "", "L", false)
		}
		for _, b := range p.Bullets {
			pdf.MultiCell(0, 6, "- "+b, "", "L", false)
		}
		pdf.Ln(2)
	}
	pdf.Ln(2)
}

// writeCertificationsPDF renders the certifications section.
func writeCertificationsPDF(pdf *fpdf.Fpdf, entries []model.CertificationEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "CERTIFICATIONS")
	for _, c := range entries {
		line := c.Name
		if c.Issuer != "" {
			line += " - " + c.Issuer
		}
		if c.Date != "" {
			line += " (" + c.Date + ")"
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
	}
	pdf.Ln(4)
}

// writeAwardsPDF renders the awards section.
func writeAwardsPDF(pdf *fpdf.Fpdf, entries []model.AwardEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "AWARDS")
	for _, a := range entries {
		line := a.Title
		if a.Date != "" {
			line += " (" + a.Date + ")"
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
		if a.Details != "" {
			pdf.MultiCell(0, 6, a.Details, "", "L", false)
		}
	}
	pdf.Ln(4)
}

// writeVolunteerPDF renders the volunteer section.
func writeVolunteerPDF(pdf *fpdf.Fpdf, entries []model.VolunteerEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "VOLUNTEER")
	for _, v := range entries {
		pdf.SetFont("Arial", "B", 10)
		line := v.Org + " | " + v.Role
		if v.StartDate != "" {
			line += " | " + v.StartDate
			if v.EndDate != "" {
				line += " - " + v.EndDate
			}
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
		pdf.SetFont("Arial", "", 10)
		for _, b := range v.Bullets {
			pdf.MultiCell(0, 6, "- "+b, "", "L", false)
		}
		pdf.Ln(2)
	}
	pdf.Ln(2)
}

// writePublicationsPDF renders the publications section.
func writePublicationsPDF(pdf *fpdf.Fpdf, entries []model.PublicationEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "PUBLICATIONS")
	for _, p := range entries {
		line := p.Title
		if p.Venue != "" {
			line += " - " + p.Venue
		}
		if p.Date != "" {
			line += " (" + p.Date + ")"
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
	}
	pdf.Ln(4)
}

// writeLanguagesPDF renders the languages section.
func writeLanguagesPDF(pdf *fpdf.Fpdf, entries []model.LanguageEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "LANGUAGES")
	for _, e := range entries {
		line := e.Name
		if e.Proficiency != "" {
			line += ": " + e.Proficiency
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
	}
	pdf.Ln(4)
}

// writeSpeakingPDF renders the speaking engagements section.
func writeSpeakingPDF(pdf *fpdf.Fpdf, entries []model.SpeakingEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "SPEAKING ENGAGEMENTS")
	for _, e := range entries {
		line := e.Title
		if e.Event != "" {
			line += " - " + e.Event
		}
		if e.Date != "" {
			line += " (" + e.Date + ")"
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
	}
	pdf.Ln(4)
}

// writeOpenSourcePDF renders the open source section.
func writeOpenSourcePDF(pdf *fpdf.Fpdf, entries []model.OpenSourceEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "OPEN SOURCE")
	for _, e := range entries {
		pdf.SetFont("Arial", "B", 10)
		line := e.Project
		if e.Role != "" {
			line += " - " + e.Role
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
		pdf.SetFont("Arial", "", 10)
		if e.Description != "" {
			pdf.MultiCell(0, 6, e.Description, "", "L", false)
		}
		if e.URL != "" {
			pdf.MultiCell(0, 6, e.URL, "", "L", false)
		}
		pdf.Ln(2)
	}
	pdf.Ln(2)
}

// writePatentsPDF renders the patents section.
func writePatentsPDF(pdf *fpdf.Fpdf, entries []model.PatentEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "PATENTS")
	for _, e := range entries {
		line := e.Title
		if e.Number != "" {
			line += " (" + e.Number + ")"
		}
		if e.Date != "" {
			line += " " + e.Date
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
	}
	pdf.Ln(4)
}

// writeInterestsPDF renders the interests section.
func writeInterestsPDF(pdf *fpdf.Fpdf, entries []model.InterestEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "INTERESTS")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Name != "" {
			names = append(names, e.Name)
		}
	}
	pdf.MultiCell(0, 6, join(names, ", "), "", "L", false)
	pdf.Ln(4)
}

// writeReferencesPDF renders the references section.
func writeReferencesPDF(pdf *fpdf.Fpdf, entries []model.ReferenceEntry) {
	if len(entries) == 0 {
		return
	}
	sectionHeading(pdf, "REFERENCES")
	for _, e := range entries {
		line := e.Name
		if e.Title != "" {
			line += ", " + e.Title
		}
		if e.Company != "" {
			line += " at " + e.Company
		}
		if e.Contact != "" {
			line += " (" + e.Contact + ")"
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
	}
	pdf.Ln(4)
}

// join concatenates strings with a separator (avoids importing strings package for one use).
func join(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
