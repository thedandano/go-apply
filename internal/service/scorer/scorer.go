package scorer

import (
	"math"
	"regexp"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/text"
)

// Service scores resumes deterministically against job descriptions.
// Implements port.Scorer. Pure computation — no I/O.
type Service struct {
	defaults *config.AppDefaults
}

var _ port.Scorer = (*Service)(nil)

// New returns a Service using the provided application defaults for all
// scoring weights and thresholds.
func New(defaults *config.AppDefaults) *Service {
	return &Service{defaults: defaults}
}

// Score computes a ScoreResult for the given resume and JD.
func (s *Service) Score(input port.ScorerInput) (model.ScoreResult, error) {
	kwScore, kwResult := s.scoreKeywordMatch(input.JD.Required, input.JD.Preferred, input.ResumeText)
	expScore := s.scoreExperienceFit(input.CandidateYears, input.RequiredYears, input.SeniorityMatch)
	impScore, metricBullets := scoreImpactEvidence(input.ResumeText, s.defaults.Scoring.ImpactBulletTarget)
	atsScore := scoreATSFormat(input.ResumeText)
	readScore, fillerPhrases := scoreReadability(input.ResumeText)

	var gaps []model.ReferenceGap
	if input.ReferenceData != nil {
		allUnmatched := make([]string, 0, len(kwResult.ReqUnmatched)+len(kwResult.PrefUnmatched))
		allUnmatched = append(allUnmatched, kwResult.ReqUnmatched...)
		allUnmatched = append(allUnmatched, kwResult.PrefUnmatched...)
		gaps = crossReferenceGaps(allUnmatched, input.ReferenceData)
	}

	return model.ScoreResult{
		ResumeLabel: input.ResumeLabel,
		ResumePath:  input.ResumePath,
		Breakdown: model.ScoreBreakdown{
			KeywordMatch:   kwScore,
			ExperienceFit:  expScore,
			ImpactEvidence: impScore,
			ATSFormat:      atsScore,
			Readability:    readScore,
		},
		Keywords:      kwResult,
		MetricBullets: metricBullets,
		FillerPhrases: fillerPhrases,
		ReferenceGaps: gaps,
	}, nil
}

// ---------------------------------------------------------------------------
// Keyword Match (45 pts)
// ---------------------------------------------------------------------------

// abbreviations maps short forms to long forms. Both directions count as a match.
// Ported from score.py ABBREVIATIONS.
var abbreviations = map[string]string{
	"aws":   "amazon web services",
	"gcp":   "google cloud platform",
	"ml":    "machine learning",
	"ai":    "artificial intelligence",
	"ci/cd": "continuous integration continuous deployment",
	"k8s":   "kubernetes",
	"js":    "javascript",
	"ts":    "typescript",
	"db":    "database",
	"api":   "application programming interface",
	"sql":   "structured query language",
	"ui":    "user interface",
	"ux":    "user experience",
	"qa":    "quality assurance",
	"oop":   "object-oriented programming",
	"nlp":   "natural language processing",
	"cv":    "computer vision",
	"etl":   "extract transform load",
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")
}

// expandAbbreviations appends both the abbreviation and its expansion to the
// text when either is present. Matches the Python expand_abbreviations logic.
func expandAbbreviations(text string) string {
	norm := normalize(text)
	for abbr, expansion := range abbreviations {
		hasAbbr := strings.Contains(norm, abbr)
		hasExpansion := strings.Contains(norm, expansion)
		if hasAbbr && !hasExpansion {
			norm += " " + expansion
		}
		if hasExpansion && !hasAbbr {
			norm += " " + abbr
		}
	}
	return norm
}

// skillMatched reports whether skill appears in resumeTextExpanded using
// word-boundary matching (no surrounding alphanumeric characters).
func skillMatched(skill, resumeTextExpanded string) bool {
	skillNorm := normalize(skill)
	resumeNorm := normalize(resumeTextExpanded)

	variants := map[string]struct{}{skillNorm: {}}
	for abbr, expansion := range abbreviations {
		if skillNorm == abbr {
			variants[expansion] = struct{}{}
		} else if skillNorm == expansion {
			variants[abbr] = struct{}{}
		}
	}

	for variant := range variants {
		pattern := `(?:[^a-z0-9]|^)` + regexp.QuoteMeta(variant) + `(?:[^a-z0-9]|$)`
		if matched, _ := regexp.MatchString(pattern, resumeNorm); matched {
			return true
		}
	}
	return false
}

func (s *Service) scoreKeywordMatch(required, preferred []string, resumeText string) (float64, model.KeywordResult) {
	expanded := expandAbbreviations(resumeText)

	var reqMatched, reqUnmatched []string
	for _, skill := range required {
		if skillMatched(skill, expanded) {
			reqMatched = append(reqMatched, skill)
		} else {
			reqUnmatched = append(reqUnmatched, skill)
		}
	}

	var prefMatched, prefUnmatched []string
	for _, skill := range preferred {
		if skillMatched(skill, expanded) {
			prefMatched = append(prefMatched, skill)
		} else {
			prefUnmatched = append(prefUnmatched, skill)
		}
	}

	var reqPct, prefPct float64
	if len(required) > 0 {
		reqPct = float64(len(reqMatched)) / float64(len(required))
	}
	if len(preferred) > 0 {
		prefPct = float64(len(prefMatched)) / float64(len(preferred))
	}

	var raw float64
	if len(preferred) == 0 {
		raw = s.defaults.Scoring.Weights.KeywordMatch * reqPct
	} else {
		base := s.defaults.Scoring.Weights.KeywordMatch * reqPct
		bonus := s.defaults.Scoring.Weights.KeywordMatch * s.defaults.Scoring.KeywordPreferredWeight * prefPct
		raw = math.Min(base+bonus, s.defaults.Scoring.Weights.KeywordMatch)
	}

	score := roundTo1(raw)
	return score, model.KeywordResult{
		ReqMatched:    nullSafe(reqMatched),
		ReqUnmatched:  nullSafe(reqUnmatched),
		PrefMatched:   nullSafe(prefMatched),
		PrefUnmatched: nullSafe(prefUnmatched),
		ReqPct:        roundTo1(reqPct * 100),
		PrefPct:       roundTo1(prefPct * 100),
	}
}

// ---------------------------------------------------------------------------
// Experience Fit (25 pts)
// ---------------------------------------------------------------------------

func (s *Service) scoreExperienceFit(candidateYears, requiredYears float64, seniorityMatch string) float64 {
	var yearsRatio float64
	if requiredYears > 0 {
		yearsRatio = math.Min(candidateYears/requiredYears, 1.0)
	} else {
		yearsRatio = 1.0
	}

	seniorityMult, ok := s.defaults.Scoring.SeniorityMultipliers[seniorityMatch]
	if !ok {
		seniorityMult = 1.0
	}

	raw := s.defaults.Scoring.Weights.ExperienceFit *
		(s.defaults.Scoring.ExperienceSeniorityWeight*seniorityMult +
			s.defaults.Scoring.ExperienceYearsWeight*yearsRatio)

	overqualified := candidateYears > s.defaults.Scoring.OverqualificationThresholdMult*requiredYears
	if overqualified {
		raw *= s.defaults.Scoring.OverqualificationPenalty
	}

	return roundTo1(raw)
}

// ---------------------------------------------------------------------------
// Impact Evidence (10 pts)
// ---------------------------------------------------------------------------

// metricPatterns matches quantitative evidence in bullet points.
// Ported from score.py METRIC_PATTERNS.
var metricPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\d+\s*%`),
	regexp.MustCompile(`(?i)\$\s*\d`),
	regexp.MustCompile(`(?i)\d+[kmb]\b`),
	regexp.MustCompile(`(?i)\d+\s*(ms|sec|seconds|minutes|hours)\b`),
	regexp.MustCompile(`(?i)\d+\s*(x|×)\s`),
	regexp.MustCompile(`(?i)\d+\+?\s*(?:\w+\s+)?(users|customers|engineers|teams|services|systems|requests|transactions|records|rows|skus|countries|markets|accounts|clients|executives|patients|members|subscribers|leads|deals|projects|campaigns|stores|locations|employees|people|partners|students|candidates|vendors|stakeholders|reports|contracts)`),
	regexp.MustCompile(`(?i)(latency|throughput|uptime|availability|p99|p95|p50)\s*\w*\s*\d`),
	regexp.MustCompile(`(?i)\d+\s*(days|weeks|months)\s*(faster|reduction|improvement|savings)`),
}

func scoreImpactEvidence(resumeText string, bulletTarget int) (float64, []string) {
	bullets := text.ExtractExperienceBullets(resumeText)
	var metricBullets []string
	for _, bullet := range bullets {
		for _, pattern := range metricPatterns {
			if pattern.MatchString(bullet) {
				metricBullets = append(metricBullets, bullet)
				break
			}
		}
	}
	count := float64(len(metricBullets))
	target := float64(bulletTarget)
	score := roundTo1(10.0 * math.Min(count/target, 1.0))
	return score, nullSafe(metricBullets)
}

// ---------------------------------------------------------------------------
// ATS Format (10 pts) — heuristic
// ---------------------------------------------------------------------------

var atsSectionHeaders = []*regexp.Regexp{
	regexp.MustCompile(`(?im)^(experience|work experience|professional experience|employment)`),
	regexp.MustCompile(`(?im)^(education|academic background)`),
	regexp.MustCompile(`(?im)^(skills|technical skills|core competencies|competencies)`),
}

func scoreATSFormat(resumeText string) float64 {
	if len(strings.TrimSpace(resumeText)) == 0 {
		return 0
	}
	found := 0
	for _, pattern := range atsSectionHeaders {
		if pattern.MatchString(resumeText) {
			found++
		}
	}
	return roundTo1(10.0 * float64(found) / float64(len(atsSectionHeaders)))
}

// ---------------------------------------------------------------------------
// Readability (10 pts) — heuristic
// ---------------------------------------------------------------------------

// fillerPhrases are weak resume phrases that reduce perceived impact.
// Ported from score.py FILLER_PHRASES.
var fillerPhrases = []string{
	"responsible for",
	"assisted with",
	"worked on",
	"helped with",
	"participated in",
}

func scoreReadability(resumeText string) (float64, []string) {
	norm := normalize(resumeText)
	var found []string
	for _, phrase := range fillerPhrases {
		if strings.Contains(norm, phrase) {
			found = append(found, phrase)
		}
	}
	// Each filler phrase costs 2 points; floor at 0.
	score := math.Max(0, 10.0-float64(len(found))*2.0)
	return roundTo1(score), nullSafe(found)
}

// ---------------------------------------------------------------------------
// Reference Gap Cross-reference
// ---------------------------------------------------------------------------

func crossReferenceGaps(unmatched []string, refData *port.ReferenceData) []model.ReferenceGap {
	if refData == nil {
		return nil
	}

	allSkillsExpanded := make(map[string]string, len(refData.AllSkills))
	for _, skill := range refData.AllSkills {
		allSkillsExpanded[skill] = expandAbbreviations(skill)
	}

	var gaps []model.ReferenceGap
	for _, jdSkill := range unmatched {
		jdExpanded := expandAbbreviations(jdSkill)
		jdNorm := normalize(jdSkill)

		for refSkill, refExpanded := range allSkillsExpanded {
			refNorm := normalize(refSkill)
			matched := jdNorm == refNorm ||
				strings.Contains(refExpanded, jdNorm) ||
				strings.Contains(jdExpanded, refNorm)
			if !matched {
				continue
			}

			gap := model.ReferenceGap{JDSkill: jdSkill, RefSkill: refSkill}
			if entry, ok := refData.PriorityMap[refSkill]; ok {
				gap.Priority = entry.Priority
				gap.Label = entry.Label
				gap.Note = entry.Note
			} else if entry, ok := refData.PriorityMap[jdSkill]; ok {
				gap.Priority = entry.Priority
				gap.Label = entry.Label
				gap.Note = entry.Note
			}
			gaps = append(gaps, gap)
			break
		}
	}
	return gaps
}

// ---------------------------------------------------------------------------
// Utilities
// ---------------------------------------------------------------------------

// roundTo1 rounds to 1 decimal place, matching Python's round(x, 1).
func roundTo1(v float64) float64 {
	return math.Round(v*10) / 10
}

// nullSafe returns an empty non-nil slice when s is nil.
func nullSafe(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
