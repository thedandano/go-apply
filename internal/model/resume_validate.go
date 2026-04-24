package model

import (
	"fmt"
	"regexp"
	"strconv"
)

var knownSections = map[string]struct{}{
	"contact":        {},
	"summary":        {},
	"experience":     {},
	"skills":         {},
	"education":      {},
	"projects":       {},
	"certifications": {},
	"awards":         {},
	"volunteer":      {},
	"publications":   {},
}

// dateRe matches YYYY or YYYY-MM
var dateRe = regexp.MustCompile(`^(\d{4})(?:-(\d{2}))?$`)

// ValidateSectionMap validates all invariants of s. Returns the first violation found.
func ValidateSectionMap(s *SectionMap) error {
	if s.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("schema_version %d: %w", s.SchemaVersion, ErrSchemaVersionUnsupported)
	}

	if s.Contact.Name == "" {
		return SchemaError{Field: "contact.name", Reason: "required"}
	}

	if s.Experience == nil {
		return SchemaError{Field: "experience", Reason: "required"}
	}

	for _, key := range s.Order {
		if _, ok := knownSections[key]; !ok {
			return SchemaError{Field: "order", Reason: fmt.Sprintf("unknown section key %q", key)}
		}
	}

	if s.Skills != nil {
		if s.Skills.Kind == SkillsKindFlat && len(s.Skills.Categorized) > 0 {
			return SchemaError{Field: "skills", Reason: "kind is flat but categorized is populated"}
		}
		if s.Skills.Kind == SkillsKindCategorized && s.Skills.Flat != "" {
			return SchemaError{Field: "skills", Reason: "kind is categorized but flat is populated"}
		}
	}

	for i, entry := range s.Experience {
		if err := validateDate(entry.StartDate); err != nil {
			return SchemaError{
				Field:  fmt.Sprintf("experience[%d].start_date", i),
				Reason: err.Error(),
			}
		}
		if entry.EndDate != "" {
			if err := validateDateOrPresent(entry.EndDate); err != nil {
				return SchemaError{
					Field:  fmt.Sprintf("experience[%d].end_date", i),
					Reason: err.Error(),
				}
			}
		}
	}

	return nil
}

func validateDate(date string) error {
	m := dateRe.FindStringSubmatch(date)
	if m == nil {
		return fmt.Errorf("invalid date format %q: must be YYYY or YYYY-MM", date)
	}
	if m[2] != "" {
		month, _ := strconv.Atoi(m[2])
		if month < 1 || month > 12 {
			return fmt.Errorf("invalid month %s in date %q", m[2], date)
		}
	}
	return nil
}

func validateDateOrPresent(date string) error {
	if date == "Present" {
		return nil
	}
	return validateDate(date)
}
