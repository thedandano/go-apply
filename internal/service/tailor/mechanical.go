package tailor

import "strings"

// BulletRewrite pairs an original bullet text with its replacement.
type BulletRewrite struct {
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
}

// ApplyBulletRewrites mechanically substitutes bullets in resumeText.
// Returns modified text and count of substitutions made.
// Empty Original entries are skipped.
func ApplyBulletRewrites(resumeText string, rewrites []BulletRewrite) (string, int) {
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
