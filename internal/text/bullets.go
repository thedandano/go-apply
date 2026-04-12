// Package text provides shared text extraction utilities used by multiple
// services (scorer, tailor). Extracting here prevents copy-paste divergence.
package text

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	expHeaders = regexp.MustCompile(
		`(?im)^(professional\s+experience|work\s+experience|experience|employment)`,
	)
	stopHeaders = regexp.MustCompile(
		`(?im)^(education|academic\s+background|skills|technical\s+skills|core\s+competencies|competencies|projects|certifications|awards|publications|volunteer|interests|summary|objective)`,
	)
)

// ExtractExperienceBullets returns all bullet-point lines from the experience
// section of the resume. If no experience header is found, the entire text is
// scanned. Matches lines starting with •, -, *, –, ▸, or ○.
func ExtractExperienceBullets(resumeText string) []string {
	section := resumeText

	expMatch := expHeaders.FindStringIndex(resumeText)
	if expMatch != nil {
		sectionStart := expMatch[1]
		rest := resumeText[sectionStart:]
		stopMatch := stopHeaders.FindStringIndex(rest)
		if stopMatch != nil {
			section = rest[:stopMatch[0]]
		} else {
			section = rest
		}
	}

	var bullets []string
	for _, line := range strings.Split(section, "\n") {
		stripped := strings.TrimSpace(line)
		if len(stripped) == 0 {
			continue
		}
		first, _ := utf8.DecodeRuneInString(stripped)
		isBullet := first == '•' || first == '-' || first == '*' ||
			first == '\u2013' || first == '\u25b8' || first == '\u25cb'
		if !isBullet {
			continue
		}
		text := strings.TrimLeftFunc(stripped, func(r rune) bool {
			return r == '•' || r == '-' || r == '*' ||
				r == '\u2013' || r == '\u25b8' || r == '\u25cb' || r == ' '
		})
		if text != "" {
			bullets = append(bullets, text)
		}
	}
	return bullets
}
