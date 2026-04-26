// Package survival computes which JD keywords survived the PDF render→extract pipeline.
package survival

import (
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

var _ port.SurvivalDiffer = (*Service)(nil)

// Service computes keyword-survival diffs with a per-instance regexp cache.
type Service struct {
	cache sync.Map // map[string]*regexp.Regexp
}

// New returns a ready-to-use Service.
func New() *Service { return &Service{} }

// Diff classifies each keyword as matched or dropped based on a case-insensitive
// whole-word search of extractedText. Assumes keywords is already deduplicated.
func (s *Service) Diff(keywords []string, extractedText string) model.KeywordSurvival {
	matched := []string{}
	dropped := []string{}

	for _, kw := range keywords {
		// Empty or whitespace-only keywords cannot match anything meaningful;
		// skip pattern compilation (which would produce `(?i)` matching everything).
		if strings.TrimSpace(kw) == "" || !s.pattern(kw).MatchString(extractedText) {
			dropped = append(dropped, kw)
		} else {
			matched = append(matched, kw)
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

// pattern returns the compiled regexp for kw, using the cache to avoid
// recompiling the same pattern across repeated Diff calls.
func (s *Service) pattern(kw string) *regexp.Regexp {
	if v, ok := s.cache.Load(kw); ok {
		return v.(*regexp.Regexp) //nolint:forcetypeassert
	}
	re := compileKeywordPattern(kw)
	s.cache.Store(kw, re)
	return re
}

// isWordChar reports whether b is a regex word character [A-Za-z0-9_].
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// compileKeywordPattern builds a case-insensitive whole-word pattern for kw.
// \b boundaries are only added where the keyword starts/ends with a word character —
// this handles keywords like "C++" and ".NET" correctly without false negatives.
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
