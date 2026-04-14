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

// metricRegex matches lines that contain a genuine metric: a number combined with
// a unit, currency symbol, multiplier, or percentage. It explicitly excludes
// version-number patterns like "Go 1.21", "Python 3.11", or "Windows 10".
//
// Strategy:
//   - A version number is: a word/identifier followed immediately by whitespace
//     and then a bare decimal (e.g. "Go 1.21", "Node 18", "Windows 10").
//   - A metric is: a percentage, a dollar/currency amount, an "Nx" multiplier,
//     a number with a clear unit suffix (k, M, B), or a large standalone integer
//     (≥ three digits — e.g. "500k users" or "1,000 requests").
var metricRegex = regexp.MustCompile(
	`(?i)` + // case-insensitive
		`(?:` +
		`\d+\.?\d*\s*%` + // percentage: 40%, 3.5%
		`|\$\s*\d[\d,\.]*` + // dollar amount: $1.2M, $50k
		`|\d[\d,\.]*\s*[kKmMbB]\b` + // magnitude suffix: 50k, 1.2M
		`|\d+x\b` + // multiplier: 2x, 10x
		`|\d{3,}` + // large standalone integer: 500, 1000
		`)`,
)

// versionRegex matches a word boundary followed by a name-like token, whitespace,
// and a bare version number — used to veto metricRegex false positives.
var versionRegex = regexp.MustCompile(`(?i)\b(?:[A-Za-z][A-Za-z0-9]*)\s+\d+(?:\.\d+)+\b`)

// atsSections are the standard ATS-readable section headers checked in ATSFormat scoring.
var atsSections = []string{"experience", "education", "skills"}

// Service implements port.Scorer.
type Service struct {
	defaults *config.AppDefaults
}

// New constructs a Service with the provided defaults.
func New(defaults *config.AppDefaults) *Service {
	return &Service{defaults: defaults}
}

// Score computes a ScoreResult for the given ScorerInput.
// Returns an error if input is nil or ResumeText is empty.
func (s *Service) Score(input *model.ScorerInput) (model.ScoreResult, error) {
	if input == nil {
		return model.ScoreResult{}, fmt.Errorf("scorer: input must not be nil")
	}
	if strings.TrimSpace(input.ResumeText) == "" {
		return model.ScoreResult{}, fmt.Errorf("scorer: ResumeText must not be empty")
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
	}, nil
}

// kwScoreResult bundles the keyword score with the structured keyword result.
type kwScoreResult struct {
	model.KeywordResult
	score float64
}

// keywordMatches reports whether keyword appears as a whole word in text
// (case-insensitive). Word-boundary matching prevents "Go" from matching
// inside "Django" or "Mongo".
func keywordMatches(text, keyword string) bool {
	pattern := `(?i)\b` + regexp.QuoteMeta(keyword) + `\b`
	ok, _ := regexp.MatchString(pattern, text)
	return ok
}

// scoreKeywords computes the KeywordMatch dimension (max 45).
// When both required and preferred keyword lists are empty, full credit is granted
// (no requirements = perfect match). When only one category is empty, it
// contributes 0 to avoid inflating the score with imaginary matches.
func (s *Service) scoreKeywords(input *model.ScorerInput) kwScoreResult {
	cfg := s.defaults.Scoring

	// No requirements at all → grant full keyword score.
	if len(input.JD.Required) == 0 && len(input.JD.Preferred) == 0 {
		return kwScoreResult{
			KeywordResult: model.KeywordResult{ReqPct: 1.0, PrefPct: 1.0},
			score:         cfg.Weights.KeywordMatch,
		}
	}

	classify := func(keywords []string) (matched, unmatched []string, pct float64) {
		if len(keywords) == 0 {
			return nil, nil, 0.0
		}
		for _, kw := range keywords {
			if keywordMatches(input.ResumeText, kw) {
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

	score := (reqPct*cfg.KeywordRequiredWeight + prefPct*cfg.KeywordPreferredWeight) * cfg.Weights.KeywordMatch

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
func (s *Service) scoreImpact(resumeText string) (float64, []string) {
	target := float64(s.defaults.Scoring.ImpactBulletTarget)
	var bullets []string

	for _, line := range strings.Split(resumeText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if metricRegex.MatchString(line) && !versionRegex.MatchString(line) {
			bullets = append(bullets, line)
		}
	}

	score := math.Min(float64(len(bullets))/target, 1.0) * s.defaults.Scoring.Weights.ImpactEvidence
	return score, bullets
}

// scoreATS computes the ATSFormat dimension (max 10) based on presence of standard
// section headers (case-insensitive).
func (s *Service) scoreATS(resumeText string) float64 {
	lower := strings.ToLower(resumeText)
	found := 0
	for _, section := range atsSections {
		if strings.Contains(lower, section) {
			found++
		}
	}
	return float64(found) / float64(len(atsSections)) * s.defaults.Scoring.Weights.ATSFormat
}

// scoreReadability computes the Readability dimension (max 10) by penalising each
// occurrence of a filler phrase in the resume. Returns the score and detected phrases.
func (s *Service) scoreReadability(resumeText string) (float64, []string) {
	cfg := s.defaults.Scoring
	lower := strings.ToLower(resumeText)

	var detected []string
	for _, phrase := range cfg.FillerPhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			detected = append(detected, phrase)
		}
	}

	score := math.Max(cfg.Weights.Readability-float64(len(detected))*cfg.ReadabilityPenaltyPerFiller, 0)
	return score, detected
}
