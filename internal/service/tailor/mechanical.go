package tailor

import (
	"strings"

	"github.com/thedandano/go-apply/internal/port"
)

// ApplySkillsRewrites applies string replacements scoped to the Skills section of resumeText.
// Returns the modified text, the entry-level substitution count (1 per matched pair regardless
// of occurrence count), and whether a Skills section was found.
// Empty Original entries are skipped. Rewrites are applied in array order (FR-007).
func ApplySkillsRewrites(resumeText string, rewrites []port.BulletRewrite) (string, int, bool) {
	section, start, end, found := ExtractSkillsSection(resumeText)
	if !found {
		return resumeText, 0, false
	}
	count := 0
	for _, rw := range rewrites {
		if rw.Original == "" {
			continue
		}
		if strings.Contains(section, rw.Original) {
			section = strings.ReplaceAll(section, rw.Original, rw.Replacement)
			count++
		}
	}
	lines := strings.Split(resumeText, "\n")
	parts := []string{
		strings.Join(lines[:start+1], "\n"),
		section,
		strings.Join(lines[end:], "\n"),
	}
	return strings.Join(parts, "\n"), count, true
}

// ApplyBulletRewrites mechanically substitutes bullets in resumeText.
// Returns modified text and count of substitutions made.
// Empty Original entries are skipped.
func ApplyBulletRewrites(resumeText string, rewrites []port.BulletRewrite) (string, int) {
	if len(rewrites) == 0 {
		return resumeText, 0
	}
	count := 0
	result := resumeText
	for _, rw := range rewrites {
		if rw.Original == "" {
			continue
		}
		if strings.Contains(result, rw.Original) {
			result = strings.ReplaceAll(result, rw.Original, rw.Replacement)
			count++
		}
	}
	return result, count
}
