// Package tailor implements the two-tier resume tailoring service.
// Tier 1 injects missing keywords into the skills section.
// Tier 2 rewrites experience bullets via LLM using accomplishments docs.
package tailor

import (
	"strings"
)

// AddKeywordsToSkillsSection finds the skills section in resumeText and appends
// an "Additional: kw1, kw2" line after the last line of that section.
// Only keywords not already present (case-insensitive) in the full resume text are added.
// Returns the modified text and the list of keywords that were added.
// If no skills section is found, returns the original text unchanged and nil.
func AddKeywordsToSkillsSection(resumeText string, missingKeywords []string) (string, []string) {
	lines := strings.Split(resumeText, "\n")

	// Filter to keywords not already in resume (case-insensitive full-text check).
	var toAdd []string
	lower := strings.ToLower(resumeText)
	for _, kw := range missingKeywords {
		if !strings.Contains(lower, strings.ToLower(kw)) {
			toAdd = append(toAdd, kw)
		}
	}
	if len(toAdd) == 0 {
		return resumeText, nil
	}

	// Find the line index of the skills section header.
	skillsIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "skills") {
			skillsIdx = i
			break
		}
	}
	if skillsIdx == -1 {
		return resumeText, nil
	}

	// Find the last line of the skills section: scan forward until a blank line
	// or end of slice.
	insertAfter := skillsIdx
	for i := skillsIdx + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			break
		}
		insertAfter = i
	}

	addLine := "Additional: " + strings.Join(toAdd, ", ")

	// Insert the new line after insertAfter.
	result := make([]string, 0, len(lines)+1)
	result = append(result, lines[:insertAfter+1]...)
	result = append(result, addLine)
	result = append(result, lines[insertAfter+1:]...)

	return strings.Join(result, "\n"), toAdd
}
