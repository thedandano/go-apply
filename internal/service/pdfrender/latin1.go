package pdfrender

import (
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/thedandano/go-apply/internal/model"
)

// transliterationMap holds explicit FR-002 mappings plus Latin chars that don't
// NFD-decompose cleanly to ASCII.
var transliterationMap = map[rune]string{
	'—': "-",             // em-dash
	'–': "-",             // en-dash
	'•': "-",             // bullet
	'‘': "'",             // left single quotation mark
	'’': "'",             // right single quotation mark
	'“': `"`,             // left double quotation mark
	'”': `"`,             // right double quotation mark
	'…': "...",           // horizontal ellipsis
	' ': " ",             // non-breaking space
	'‐': "-",             // hyphen
	'‑': "-",             // non-breaking hyphen
	'ß': "ss",            // ß
	'Æ': "AE", 'æ': "ae", // Æ / æ
	'Œ': "OE", 'œ': "oe", // Œ / œ
	'Ø': "O", 'ø': "o", // Ø / ø
	'Þ': "TH", 'þ': "th", // Þ / þ
	'Ð': "D", 'ð': "d", // Ð / ð
}

// transliterateField converts a single string field to ASCII. Explicit mappings take
// priority; then NFD decomposition extracts the base letter; runes > U+00FF with no
// mapping fall back to '?' with a slog.Warn containing codepoint and field only
// (no surrounding text — PII protection).
func transliterateField(value, fieldName string) string {
	if value == "" {
		return value
	}
	// Fast path: nothing to do if all bytes are ASCII.
	allASCII := true
	for i := 0; i < len(value); i++ {
		if value[i] > 0x7F {
			allASCII = false
			break
		}
	}
	if allASCII {
		return value
	}

	var buf strings.Builder
	buf.Grow(len(value))
	for _, r := range value {
		if r <= 0x7F {
			buf.WriteRune(r)
			continue
		}
		if mapped, ok := transliterationMap[r]; ok {
			buf.WriteString(mapped)
			continue
		}
		// NFD decomposition: strip combining marks, keep first base rune.
		nfd := norm.NFD.String(string(r))
		base := rune(0)
		for _, nr := range nfd {
			if !unicode.Is(unicode.Mn, nr) {
				base = nr
				break
			}
		}
		if base != 0 && base <= 0x7F {
			buf.WriteRune(base)
			continue
		}
		// Unrepresentable: substitute '?' and warn (only for runes above Latin-1 range).
		buf.WriteRune('?')
		if r > 0xFF {
			slog.Warn("transliterate: unrepresentable character",
				"codepoint", fmt.Sprintf("U+%04X", r),
				"field", fieldName)
		}
	}
	return buf.String()
}

// transliterateLatin1 returns a deep copy of sections with every string field
// transliterated to ASCII. The original *model.SectionMap is never mutated.
func transliterateLatin1(sections *model.SectionMap) model.SectionMap {
	tf := transliterateField

	out := model.SectionMap{
		SchemaVersion: sections.SchemaVersion,
		Order:         append([]string(nil), sections.Order...),
	}

	// Contact
	out.Contact = model.ContactInfo{
		Name:     tf(sections.Contact.Name, "contact.name"),
		Email:    tf(sections.Contact.Email, "contact.email"),
		Phone:    tf(sections.Contact.Phone, "contact.phone"),
		Location: tf(sections.Contact.Location, "contact.location"),
	}
	if len(sections.Contact.Links) > 0 {
		out.Contact.Links = make([]string, len(sections.Contact.Links))
		for i, l := range sections.Contact.Links {
			out.Contact.Links[i] = tf(l, fmt.Sprintf("contact.links[%d]", i))
		}
	}

	// Summary
	out.Summary = tf(sections.Summary, "summary")

	// Experience
	if len(sections.Experience) > 0 {
		out.Experience = make([]model.ExperienceEntry, len(sections.Experience))
		for i, e := range sections.Experience {
			p := fmt.Sprintf("experience[%d]", i)
			ne := model.ExperienceEntry{
				Company:   tf(e.Company, p+".company"),
				Role:      tf(e.Role, p+".role"),
				StartDate: tf(e.StartDate, p+".start_date"),
				EndDate:   tf(e.EndDate, p+".end_date"),
				Location:  tf(e.Location, p+".location"),
			}
			if len(e.Bullets) > 0 {
				ne.Bullets = make([]string, len(e.Bullets))
				for j, b := range e.Bullets {
					ne.Bullets[j] = tf(b, fmt.Sprintf("%s.bullets[%d]", p, j))
				}
			}
			out.Experience[i] = ne
		}
	}

	// Education
	if len(sections.Education) > 0 {
		out.Education = make([]model.EducationEntry, len(sections.Education))
		for i, e := range sections.Education {
			p := fmt.Sprintf("education[%d]", i)
			out.Education[i] = model.EducationEntry{
				School:    tf(e.School, p+".school"),
				Degree:    tf(e.Degree, p+".degree"),
				StartDate: tf(e.StartDate, p+".start_date"),
				EndDate:   tf(e.EndDate, p+".end_date"),
				Location:  tf(e.Location, p+".location"),
				Details:   tf(e.Details, p+".details"),
			}
		}
	}

	// Skills
	if sections.Skills != nil {
		ns := &model.SkillsSection{
			Kind: sections.Skills.Kind,
			Flat: tf(sections.Skills.Flat, "skills.flat"),
		}
		if len(sections.Skills.Categorized) > 0 {
			ns.Categorized = make(map[string][]string, len(sections.Skills.Categorized))
			for cat, vals := range sections.Skills.Categorized {
				tcat := tf(cat, "skills.categorized.key:"+cat)
				tvals := make([]string, len(vals))
				for j, v := range vals {
					tvals[j] = tf(v, fmt.Sprintf("skills.categorized[%s][%d]", cat, j))
				}
				ns.Categorized[tcat] = tvals
			}
		}
		out.Skills = ns
	}

	// Projects
	if len(sections.Projects) > 0 {
		out.Projects = make([]model.ProjectEntry, len(sections.Projects))
		for i, p2 := range sections.Projects {
			p := fmt.Sprintf("projects[%d]", i)
			np := model.ProjectEntry{
				Name:        tf(p2.Name, p+".name"),
				Description: tf(p2.Description, p+".description"),
				URL:         tf(p2.URL, p+".url"),
			}
			if len(p2.Bullets) > 0 {
				np.Bullets = make([]string, len(p2.Bullets))
				for j, b := range p2.Bullets {
					np.Bullets[j] = tf(b, fmt.Sprintf("%s.bullets[%d]", p, j))
				}
			}
			out.Projects[i] = np
		}
	}

	// Certifications
	if len(sections.Certifications) > 0 {
		out.Certifications = make([]model.CertificationEntry, len(sections.Certifications))
		for i, e := range sections.Certifications {
			p := fmt.Sprintf("certifications[%d]", i)
			out.Certifications[i] = model.CertificationEntry{
				Name:   tf(e.Name, p+".name"),
				Issuer: tf(e.Issuer, p+".issuer"),
				Date:   tf(e.Date, p+".date"),
			}
		}
	}

	// Awards
	if len(sections.Awards) > 0 {
		out.Awards = make([]model.AwardEntry, len(sections.Awards))
		for i, e := range sections.Awards {
			p := fmt.Sprintf("awards[%d]", i)
			out.Awards[i] = model.AwardEntry{
				Title:   tf(e.Title, p+".title"),
				Date:    tf(e.Date, p+".date"),
				Details: tf(e.Details, p+".details"),
			}
		}
	}

	// Volunteer
	if len(sections.Volunteer) > 0 {
		out.Volunteer = make([]model.VolunteerEntry, len(sections.Volunteer))
		for i, e := range sections.Volunteer {
			p := fmt.Sprintf("volunteer[%d]", i)
			ne := model.VolunteerEntry{
				Org:       tf(e.Org, p+".org"),
				Role:      tf(e.Role, p+".role"),
				StartDate: tf(e.StartDate, p+".start_date"),
				EndDate:   tf(e.EndDate, p+".end_date"),
			}
			if len(e.Bullets) > 0 {
				ne.Bullets = make([]string, len(e.Bullets))
				for j, b := range e.Bullets {
					ne.Bullets[j] = tf(b, fmt.Sprintf("%s.bullets[%d]", p, j))
				}
			}
			out.Volunteer[i] = ne
		}
	}

	// Publications
	if len(sections.Publications) > 0 {
		out.Publications = make([]model.PublicationEntry, len(sections.Publications))
		for i, e := range sections.Publications {
			p := fmt.Sprintf("publications[%d]", i)
			out.Publications[i] = model.PublicationEntry{
				Title: tf(e.Title, p+".title"),
				Venue: tf(e.Venue, p+".venue"),
				Date:  tf(e.Date, p+".date"),
				URL:   tf(e.URL, p+".url"),
			}
		}
	}

	// Languages
	if len(sections.Languages) > 0 {
		out.Languages = make([]model.LanguageEntry, len(sections.Languages))
		for i, e := range sections.Languages {
			p := fmt.Sprintf("languages[%d]", i)
			out.Languages[i] = model.LanguageEntry{
				Name:        tf(e.Name, p+".name"),
				Proficiency: tf(e.Proficiency, p+".proficiency"),
			}
		}
	}

	// Speaking
	if len(sections.Speaking) > 0 {
		out.Speaking = make([]model.SpeakingEntry, len(sections.Speaking))
		for i, e := range sections.Speaking {
			p := fmt.Sprintf("speaking[%d]", i)
			out.Speaking[i] = model.SpeakingEntry{
				Title: tf(e.Title, p+".title"),
				Event: tf(e.Event, p+".event"),
				Date:  tf(e.Date, p+".date"),
				URL:   tf(e.URL, p+".url"),
			}
		}
	}

	// OpenSource
	if len(sections.OpenSource) > 0 {
		out.OpenSource = make([]model.OpenSourceEntry, len(sections.OpenSource))
		for i, e := range sections.OpenSource {
			p := fmt.Sprintf("open_source[%d]", i)
			out.OpenSource[i] = model.OpenSourceEntry{
				Project:     tf(e.Project, p+".project"),
				Role:        tf(e.Role, p+".role"),
				URL:         tf(e.URL, p+".url"),
				Description: tf(e.Description, p+".description"),
			}
		}
	}

	// Patents
	if len(sections.Patents) > 0 {
		out.Patents = make([]model.PatentEntry, len(sections.Patents))
		for i, e := range sections.Patents {
			p := fmt.Sprintf("patents[%d]", i)
			out.Patents[i] = model.PatentEntry{
				Title:  tf(e.Title, p+".title"),
				Number: tf(e.Number, p+".number"),
				Date:   tf(e.Date, p+".date"),
				URL:    tf(e.URL, p+".url"),
			}
		}
	}

	// Interests
	if len(sections.Interests) > 0 {
		out.Interests = make([]model.InterestEntry, len(sections.Interests))
		for i, e := range sections.Interests {
			out.Interests[i] = model.InterestEntry{
				Name: tf(e.Name, fmt.Sprintf("interests[%d].name", i)),
			}
		}
	}

	// References
	if len(sections.References) > 0 {
		out.References = make([]model.ReferenceEntry, len(sections.References))
		for i, e := range sections.References {
			p := fmt.Sprintf("references[%d]", i)
			out.References[i] = model.ReferenceEntry{
				Name:    tf(e.Name, p+".name"),
				Title:   tf(e.Title, p+".title"),
				Company: tf(e.Company, p+".company"),
				Contact: tf(e.Contact, p+".contact"),
			}
		}
	}

	return out
}
