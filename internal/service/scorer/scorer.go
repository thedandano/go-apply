// Package scorer computes a numeric score for a resume against a job description.
// It is pure Go with no I/O — deterministic given the same inputs.
package scorer

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Scorer = (*Service)(nil)

// metricRegex matches lines that contain a genuine quantified metric.
// Each branch is a distinct signal type:
//   - percentage:       "reduced latency by 40%"
//   - dollar amount:    "saved $1.2M annually"
//   - magnitude suffix: "handled 50k requests/sec"
//   - multiplier:       "improved throughput by 2x"
//   - large integer:    "served 500 daily users"
//
// Calendar years and version strings are stripped from each line BEFORE this
// regex is applied (see scoreImpact), so "2023" and "Go 1.21" never reach here.
var metricRegex = regexp.MustCompile(
	`(?i)` +
		`(?:` +
		`\d+\.?\d*\s*%` + // percentage: 40%, 3.5%
		`|\$\s*\d[\d,\.]*` + // dollar amount: $1.2M, $50k
		`|\d[\d,\.]*\s*[kKmMbB]\b` + // magnitude suffix: 50k, 1.2M
		`|\d+x\b` + // multiplier: 2x, 10x
		`|\d{3,}` + // large integer: 500, 1000 (years already stripped)
		`)`,
)

// versionRegex identifies software/tool version patterns like "Go 1.21",
// "Python 3.11", or "Node 18.0". Stripped from lines before metric detection.
var versionRegex = regexp.MustCompile(`(?i)\b[A-Za-z][A-Za-z0-9]*\s+\d+(?:\.\d+)+\b`)

// yearRegex matches bare 4-digit calendar years (1900–2099).
// Stripped from lines before metric detection to prevent date ranges like
// "Jan 2019 – Dec 2023" from counting as impact bullets.
var yearRegex = regexp.MustCompile(`\b(?:19|20)\d{2}\b`)

// atsSectionPatterns matches common ATS section headers as standalone lines.
// A line qualifies when, after stripping leading/trailing whitespace and
// optional trailing punctuation, it equals a known header (with common prefixes).
// This prevents body-text false positives like "5 years of experience in Go".
var atsSectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*(?:work\s+|professional\s+)?experience\s*:?\s*$`),
	regexp.MustCompile(`(?i)^\s*(?:academic\s+)?education\s*:?\s*$`),
	regexp.MustCompile(`(?i)^\s*(?:technical\s+|core\s+)?skills?\s*:?\s*$`),
}

// Service implements port.Scorer.
type Service struct {
	defaults       *config.AppDefaults
	fillerPatterns []*regexp.Regexp // pre-compiled from defaults.Scoring.FillerPhrases
}

// New constructs a Service. Filler-phrase patterns are pre-compiled once at
// construction time — they depend on config but are fixed for the lifetime of
// the service.
func New(defaults *config.AppDefaults) *Service {
	patterns := make([]*regexp.Regexp, len(defaults.Scoring.FillerPhrases))
	for i, phrase := range defaults.Scoring.FillerPhrases {
		// Filler phrases consist entirely of word characters and spaces, so
		// \b boundaries are valid on both sides.
		patterns[i] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(phrase) + `\b`)
	}
	return &Service{defaults: defaults, fillerPatterns: patterns}
}

// Score computes a ScoreResult for the given ScorerInput.
// Returns an error if input is nil, ResumeText is empty, or SeniorityMatch is
// not a recognised key in the configured seniority multiplier map.
func (s *Service) Score(input *model.ScorerInput) (model.ScoreResult, error) {
	if input == nil {
		return model.ScoreResult{}, fmt.Errorf("scorer: input must not be nil")
	}
	if strings.TrimSpace(input.ResumeText) == "" {
		return model.ScoreResult{}, fmt.Errorf("scorer: ResumeText must not be empty")
	}
	if _, ok := s.defaults.Scoring.SeniorityMultipliers[input.SeniorityMatch]; !ok {
		valid := make([]string, 0, len(s.defaults.Scoring.SeniorityMultipliers))
		for k := range s.defaults.Scoring.SeniorityMultipliers {
			valid = append(valid, `"`+k+`"`)
		}
		return model.ScoreResult{}, fmt.Errorf(
			"scorer: unknown SeniorityMatch %q — valid values: %s",
			input.SeniorityMatch, strings.Join(valid, ", "),
		)
	}

	kwResult := s.scoreKeywords(input)
	expScore := s.scoreExperience(input)
	impactScore, metricBullets := s.scoreImpact(input.ResumeText)
	atsScore := s.scoreATS(input.ResumeText)
	readScore, fillerPhrases := s.scoreReadability(input.ResumeText)

	return model.ScoreResult{
		ResumeLabel: input.ResumeLabel,
		ResumePath:  input.ResumePath,
		Breakdown: model.ScoreBreakdown{
			KeywordMatch:   kwResult.score,
			ExperienceFit:  expScore,
			ImpactEvidence: impactScore,
			ATSFormat:      atsScore,
			Readability:    readScore,
		},
		Keywords:      kwResult.KeywordResult,
		MetricBullets: metricBullets,
		FillerPhrases: fillerPhrases,
		ReferenceGaps: referenceGaps(input.ReferenceData),
	}, nil
}

// referenceGaps extracts the gap slice from ReferenceData for passthrough onto
// ScoreResult. Returns nil when ReferenceData is nil or its map is empty.
func referenceGaps(rd *model.ReferenceData) []model.ReferenceGap {
	if rd == nil || len(rd.PriorityMap) == 0 {
		return nil
	}
	gaps := make([]model.ReferenceGap, 0, len(rd.PriorityMap))
	for _, g := range rd.PriorityMap {
		gaps = append(gaps, g)
	}
	return gaps
}

// kwScoreResult bundles the keyword score with the structured keyword result.
type kwScoreResult struct {
	model.KeywordResult
	score float64
}

// isWordChar reports whether b is a regex word character [A-Za-z0-9_].
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// compileKeywordPattern builds a case-insensitive whole-word pattern for kw.
// \b boundaries are added only at positions where the keyword character is a
// word char — this correctly handles keywords like "C++", "C#", and ".NET"
// where a trailing \b after '+' or '#' would never fire.
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

// scoreKeywords computes the KeywordMatch dimension (max 45).
//
// Weight distribution:
//   - Both required and preferred present: configured weights (0.7 / 0.3)
//   - Only one category present: that category gets full weight (1.0)
//   - Both empty: full score granted (no requirements = perfect match)
func (s *Service) scoreKeywords(input *model.ScorerInput) kwScoreResult {
	cfg := s.defaults.Scoring

	if len(input.JD.Required) == 0 && len(input.JD.Preferred) == 0 {
		return kwScoreResult{
			KeywordResult: model.KeywordResult{ReqPct: 1.0, PrefPct: 1.0},
			score:         cfg.Weights.KeywordMatch,
		}
	}

	// Determine effective weights based on which lists are populated.
	reqW, prefW := cfg.KeywordRequiredWeight, cfg.KeywordPreferredWeight
	switch {
	case len(input.JD.Required) == 0:
		reqW, prefW = 0.0, 1.0
	case len(input.JD.Preferred) == 0:
		reqW, prefW = 1.0, 0.0
	}

	classify := func(keywords []string) (matched, unmatched []string, pct float64) {
		if len(keywords) == 0 {
			return nil, nil, 0.0
		}
		for _, kw := range keywords {
			pat := compileKeywordPattern(kw)
			if pat.MatchString(input.ResumeText) {
				matched = append(matched, kw)
			} else {
				unmatched = append(unmatched, kw)
			}
		}
		pct = float64(len(matched)) / float64(len(keywords))
		return
	}

	reqMatched, reqUnmatched, reqPct := classify(input.JD.Required)
	prefMatched, prefUnmatched, prefPct := classify(input.JD.Preferred)
	score := (reqPct*reqW + prefPct*prefW) * cfg.Weights.KeywordMatch

	return kwScoreResult{
		KeywordResult: model.KeywordResult{
			ReqMatched:    reqMatched,
			ReqUnmatched:  reqUnmatched,
			PrefMatched:   prefMatched,
			PrefUnmatched: prefUnmatched,
			ReqPct:        reqPct,
			PrefPct:       prefPct,
		},
		score: score,
	}
}

// scoreExperience computes the ExperienceFit dimension (max 25).
func (s *Service) scoreExperience(input *model.ScorerInput) float64 {
	cfg := s.defaults.Scoring

	yearsScore := 1.0
	if input.RequiredYears > 0 {
		yearsScore = math.Min(input.CandidateYears/input.RequiredYears, 1.0)
		if input.CandidateYears > input.RequiredYears*cfg.OverqualificationThresholdMult {
			yearsScore *= cfg.OverqualificationPenalty
		}
	}

	seniorityScore := cfg.SeniorityMultipliers[input.SeniorityMatch]
	return (yearsScore*cfg.ExperienceYearsWeight + seniorityScore*cfg.ExperienceSeniorityWeight) * cfg.Weights.ExperienceFit
}

// scoreImpact computes the ImpactEvidence dimension (max 10) and returns the
// matched metric bullet lines.
//
// Version strings (e.g. "Python 3.11") are stripped from each line before
// checking for metrics — this prevents a line like "reduced latency by 40%
// migrating from Python 2.7 to 3.11" from being discarded just because it
// also mentions a version.
func (s *Service) scoreImpact(resumeText string) (float64, []string) {
	target := float64(s.defaults.Scoring.ImpactBulletTarget)
	var bullets []string

	for _, line := range strings.Split(resumeText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip version strings and calendar years, then check for genuine metrics.
		stripped := versionRegex.ReplaceAllString(line, "")
		stripped = yearRegex.ReplaceAllString(stripped, "")
		if metricRegex.MatchString(stripped) {
			bullets = append(bullets, line) // preserve original text in output
		}
	}

	score := math.Min(float64(len(bullets))/target, 1.0) * s.defaults.Scoring.Weights.ImpactEvidence
	return score, bullets
}

// scoreATS computes the ATSFormat dimension (max 10) based on presence of
// standard ATS section headers as standalone lines. Each section pattern
// checks one line at a time to avoid false positives from body text.
func (s *Service) scoreATS(resumeText string) float64 {
	lines := strings.Split(resumeText, "\n")
	found := 0
	for _, pat := range atsSectionPatterns {
		for _, line := range lines {
			if pat.MatchString(line) {
				found++
				break
			}
		}
	}
	return float64(found) / float64(len(atsSectionPatterns)) * s.defaults.Scoring.Weights.ATSFormat
}

// scoreReadability computes the Readability dimension (max 10) by penalising
// each filler phrase that appears in the resume. Uses pre-compiled word-boundary
// patterns to avoid false positives (e.g. "worked on" inside "networked on").
func (s *Service) scoreReadability(resumeText string) (float64, []string) {
	cfg := s.defaults.Scoring
	var detected []string
	for i, pat := range s.fillerPatterns {
		if pat.MatchString(resumeText) {
			detected = append(detected, cfg.FillerPhrases[i])
		}
	}
	score := math.Max(cfg.Weights.Readability-float64(len(detected))*cfg.ReadabilityPenaltyPerFiller, 0)
	return score, detected
}
