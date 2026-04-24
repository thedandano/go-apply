// Package tailor provides the two-tier resume tailoring service.
package tailor

import (
	"regexp"
	"strings"
)

// skillsHeaderRe matches common Skills section headers:
// "## Skills", "Skills:", "SKILLS", "## Technical Skills", "Core Skills:", etc.
var skillsHeaderRe = regexp.MustCompile(`(?im)^(#{0,3}\s*)((technical|core|key|professional|additional)?\s*skills[:\s]*)$`)

// ExtractSkillsSection locates the Skills section in resumeText and returns its body text
// (lines after the header line), the line index of the header (start), the exclusive end
// index (end), and whether a Skills header was found. The caller can splice a modified body
// back with: strings.Join(lines[:start+1], "\n") + "\n" + newBody + "\n" + strings.Join(lines[end:], "\n").
func ExtractSkillsSection(resumeText string) (section string, start, end int, found bool) {
	lines := strings.Split(resumeText, "\n")
	start = -1
	for i, line := range lines {
		if skillsHeaderRe.MatchString(strings.TrimSpace(line)) {
			start = i
			break
		}
	}
	if start == -1 {
		return "", 0, 0, false
	}
	end = len(lines)
	for i := start + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if isHeaderLine(trimmed) && !skillsHeaderRe.MatchString(trimmed) {
			end = i
			break
		}
	}
	return strings.Join(lines[start+1:end], "\n"), start, end, true
}

// AddKeywordsToSkillsSection injects missing keywords into the Skills section of resumeText.
// Deduplication is case-insensitive — keywords already present anywhere in the Skills section
// are not re-added. Returns the modified resume text and the list of keywords that were added.
// When no Skills section is found, the original text is returned unchanged with an empty slice.
func AddKeywordsToSkillsSection(resumeText string, keywords []string) (string, []string) {
	lines := strings.Split(resumeText, "\n")

	skillsStart := -1
	for i, line := range lines {
		if skillsHeaderRe.MatchString(strings.TrimSpace(line)) {
			skillsStart = i
			break
		}
	}

	if skillsStart == -1 {
		return resumeText, nil
	}

	// Find the end of the Skills section: next header or end of file.
	skillsEnd := len(lines)
	for i := skillsStart + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if isHeaderLine(trimmed) && !skillsHeaderRe.MatchString(trimmed) {
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
		return resumeText, nil
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

	return strings.Join(newLines, "\n"), toAdd
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
