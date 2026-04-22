// Package tailor provides the two-tier resume tailoring service.
package tailor

import (
	"strings"
)

// isSkillsHeaderLine returns true when a trimmed line is a section header that
// mentions "skills". Delegates to isHeaderLine for header-shape detection, then
// filters to skills-specific headers. Handles variants like "SKILLS & ABILITIES",
// "Key Skills:", "## Technical Skills", "SKILLS AND ABILITIES".
func isSkillsHeaderLine(trimmed string) bool {
	if !isHeaderLine(trimmed) {
		return false
	}
	return strings.Contains(strings.ToLower(trimmed), "skills")
}

// AddKeywordsToSkillsSection injects missing keywords into the Skills section of resumeText.
// Deduplication is case-insensitive — keywords already present anywhere in the Skills section
// are not re-added. Returns the modified resume text, the list of keywords that were added,
// and a boolean indicating whether a Skills section header was found. The boolean lets callers
// distinguish "no section found" (blocker — nothing was attempted) from "section found but
// every keyword already present" (success — nothing to do).
func AddKeywordsToSkillsSection(resumeText string, keywords []string) (string, []string, bool) {
	lines := strings.Split(resumeText, "\n")

	skillsStart := -1
	for i, line := range lines {
		if isSkillsHeaderLine(strings.TrimSpace(line)) {
			skillsStart = i
			break
		}
	}

	if skillsStart == -1 {
		return resumeText, nil, false
	}

	// Find the end of the Skills section: next header or end of file.
	skillsEnd := len(lines)
	for i := skillsStart + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if isHeaderLine(trimmed) && !isSkillsHeaderLine(trimmed) {
			skillsEnd = i
			break
		}
	}

	// Collect existing content in Skills section for dedup check.
	sectionContent := strings.Join(lines[skillsStart:skillsEnd], "\n")
	sectionLower := strings.ToLower(sectionContent)

	var toAdd []string
	for _, kw := range keywords {
		if !strings.Contains(sectionLower, strings.ToLower(kw)) {
			toAdd = append(toAdd, kw)
		}
	}

	if len(toAdd) == 0 {
		return resumeText, nil, true
	}

	// Insert the new keywords on a line just before the end of the Skills section.
	// If the line before skillsEnd is empty, insert before that blank line.
	insertAt := skillsEnd
	for insertAt > skillsStart+1 && strings.TrimSpace(lines[insertAt-1]) == "" {
		insertAt--
	}

	injection := strings.Join(toAdd, ", ")
	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, injection)
	newLines = append(newLines, lines[insertAt:]...)

	return strings.Join(newLines, "\n"), toAdd, true
}

// knownSectionKeywords is the set of lowercase first-words recognized as resume section headers.
var knownSectionKeywords = map[string]bool{
	"skills":         true,
	"experience":     true,
	"education":      true,
	"projects":       true,
	"summary":        true,
	"objective":      true,
	"awards":         true,
	"certifications": true,
	"languages":      true,
	"interests":      true,
	"publications":   true,
	"volunteer":      true,
	"references":     true,
	"work":           true,
	"professional":   true,
	"technical":      true,
	"core":           true,
	"additional":     true,
	"key":            true,
}

// knownCompoundHeaders is the set of lowercase full-line strings recognized as compound section headers.
var knownCompoundHeaders = map[string]bool{
	"work experience":         true,
	"technical skills":        true,
	"professional experience": true,
	"core competencies":       true,
	"additional information":  true,
}

// isHeaderLine returns true when a trimmed line looks like a resume section header.
// It uses a known-section-names approach to avoid false positives like company names
// (e.g. "ACME CORP") or abbreviated dates (e.g. "JAN 2020").
func isHeaderLine(trimmed string) bool {
	// Bullet lines are never headers.
	if isBulletLine(trimmed) {
		return false
	}
	// Markdown-style headers: "# Skills", "## Experience", etc.
	if strings.HasPrefix(trimmed, "#") {
		return true
	}
	// Plain section titles ending with a colon, e.g. "Experience:", "EXPERIENCE:".
	// Require no sentence-internal punctuation, digits, or commas to avoid matching prose.
	if strings.HasSuffix(trimmed, ":") {
		inner := trimmed[:len(trimmed)-1]
		if !strings.ContainsAny(inner, ".,;!?()[]{}\"") && !strings.ContainsAny(inner, "0123456789") {
			lower := strings.ToLower(strings.TrimSpace(inner))
			firstWord := strings.Fields(lower)
			if len(firstWord) > 0 && knownSectionKeywords[firstWord[0]] {
				return true
			}
		}
	}
	// Match by known first word or known compound header (case-insensitive).
	lower := strings.ToLower(trimmed)
	// Check exact compound header match first.
	if knownCompoundHeaders[lower] {
		return true
	}
	// Check that the first word is a known section keyword.
	fields := strings.Fields(lower)
	if len(fields) > 0 && knownSectionKeywords[fields[0]] {
		return true
	}
	return false
}
