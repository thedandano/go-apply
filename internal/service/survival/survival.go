// Package survival computes which JD keywords survived the PDF render→extract pipeline.
package survival

import (
	"log/slog"
	"regexp"

	"github.com/thedandano/go-apply/internal/model"
)

// Service computes keyword-survival diffs.
type Service struct{}

// New returns a ready-to-use Service.
func New() *Service { return &Service{} }

// Diff classifies each keyword as matched or dropped based on a case-insensitive
// whole-word search of extractedText. Assumes keywords is already deduplicated.
func (s *Service) Diff(keywords []string, extractedText string) model.KeywordSurvival {
	matched := []string{}
	dropped := []string{}

	for _, kw := range keywords {
		if compileKeywordPattern(kw).MatchString(extractedText) {
			matched = append(matched, kw)
		} else {
			dropped = append(dropped, kw)
		}
	}

	slog.Info("survival.diff",
		"total", len(keywords),
		"dropped", len(dropped),
		"matched", len(matched),
	)

	return model.KeywordSurvival{
		Dropped:         dropped,
		Matched:         matched,
		TotalJDKeywords: len(keywords),
	}
}

// isWordChar reports whether b is a regex word character [A-Za-z0-9_].
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// compileKeywordPattern builds a case-insensitive whole-word pattern for kw.
// \b boundaries are only added where the keyword character is a word char —
// this handles keywords like "C++" and ".NET" correctly.
func compileKeywordPattern(kw string) *regexp.Regexp {
	quoted := regexp.QuoteMeta(kw)
	prefix, suffix := "", ""
	if len(kw) > 0 && isWordChar(kw[0]) {
		prefix = `\b`
	}
	if len(kw) > 0 && isWordChar(kw[len(kw)-1]) {
		suffix = `\b`
	}
	return regexp.MustCompile(`(?i)` + prefix + quoted + suffix)
}
