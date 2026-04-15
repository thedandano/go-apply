package scorer_test

import (
	"math"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

// within asserts that got is within tolerance of want.
func within(t *testing.T, label string, want, got, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: want %.4f, got %.4f (tolerance %.4f)", label, want, got, tol)
	}
}

func defaults() *config.AppDefaults {
	return config.EmbeddedDefaults()
}

// baseInput returns a valid ScorerInput with all required fields populated.
// SeniorityMatch is intentionally set to a valid value — an empty/missing
// SeniorityMatch is an error (tested separately).
func baseInput() model.ScorerInput {
	return model.ScorerInput{
		ResumeText:     "Experience\nEducation\nSkills\nBuilt payment system reducing latency by 40%.\nImplemented caching layer cutting costs by $50k/year.",
		ResumeLabel:    "test-resume",
		ResumePath:     "/tmp/test.pdf",
		JD:             model.JDData{Required: []string{"Go", "PostgreSQL"}, Preferred: []string{"Kubernetes"}},
		CandidateYears: 5,
		RequiredYears:  5,
		SeniorityMatch: "exact",
	}
}

// ── KeywordMatch ──────────────────────────────────────────────────────────────

func TestScore_AllKeywordsMatched(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\nGo PostgreSQL Kubernetes developer"
	input.JD.Required = []string{"Go", "PostgreSQL"}
	input.JD.Preferred = []string{"Kubernetes"}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// reqPct=1.0, prefPct=1.0, both lists populated → (0.7+0.3)*45 = 45
	within(t, "KeywordMatch", 45.0, result.Breakdown.KeywordMatch, 0.01)
	within(t, "ReqPct", 1.0, result.Keywords.ReqPct, 0.01)
	within(t, "PrefPct", 1.0, result.Keywords.PrefPct, 0.01)
}

func TestScore_NoKeywordsMatched(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\nPython Django developer"
	input.JD.Required = []string{"Go", "PostgreSQL"}
	input.JD.Preferred = []string{"Kubernetes"}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "KeywordMatch", 0.0, result.Breakdown.KeywordMatch, 0.01)
	within(t, "ReqPct", 0.0, result.Keywords.ReqPct, 0.01)
}

func TestScore_PartialKeywordsMatched(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\nGo developer with PostgreSQL experience"
	input.JD.Required = []string{"Go", "PostgreSQL", "Redis"}
	input.JD.Preferred = []string{"Kubernetes", "Docker"}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// req: 2/3 matched, pref: 0/2 matched
	// (2/3 * 0.7 + 0/2 * 0.3) * 45 = 21.0
	within(t, "KeywordMatch", 21.0, result.Breakdown.KeywordMatch, 0.1)
	within(t, "ReqPct", 2.0/3.0, result.Keywords.ReqPct, 0.01)
}

func TestScore_KeywordMatchCaseInsensitive(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\ngo postgresql developer"
	input.JD.Required = []string{"Go", "PostgreSQL"}
	input.JD.Preferred = []string{} // no preferred

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only required present → full weight (1.0) on required.
	// reqPct=1.0 → 1.0 * 1.0 * 45 = 45.0
	within(t, "KeywordMatch", 45.0, result.Breakdown.KeywordMatch, 0.1)
}

func TestScore_NoJDKeywords_ReturnsError(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.JD = model.JDData{} // fully empty JD — no keywords, no title, no company

	_, err := svc.Score(&input)
	if err == nil {
		t.Fatal("expected error for empty JD, got nil")
	}
}

func TestScore_OnlyPreferredKeywords_FullWeightOnPreferred(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\nKubernetes developer"
	input.JD.Required = []string{} // no required
	input.JD.Preferred = []string{"Kubernetes", "Docker"}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only preferred present → full weight (1.0) on preferred.
	// prefPct = 1/2 = 0.5 → 0.5 * 1.0 * 45 = 22.5
	within(t, "KeywordMatch", 22.5, result.Breakdown.KeywordMatch, 0.1)
}

// BUG FIX: "Go" must not match inside "Django" (word-boundary matching).
func TestScore_KeywordWordBoundary_NoCrossTalent(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\nDjango developer"
	input.JD.Required = []string{"Go"}
	input.JD.Preferred = []string{}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "KeywordMatch", 0.0, result.Breakdown.KeywordMatch, 0.01)
}

// BUG FIX: Keywords containing non-word chars (C++, C#, .NET) must match correctly.
func TestScore_KeywordWithNonWordChars_CppMatches(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\nC++ developer with 5 years"
	input.JD.Required = []string{"C++"}
	input.JD.Preferred = []string{}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "KeywordMatch", 45.0, result.Breakdown.KeywordMatch, 0.01)
}

func TestScore_KeywordWithNonWordChars_DotNetMatches(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\nASP.NET developer"
	input.JD.Required = []string{".NET"}
	input.JD.Preferred = []string{}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "KeywordMatch", 45.0, result.Breakdown.KeywordMatch, 0.01)
}

// ── ExperienceFit ─────────────────────────────────────────────────────────────

func TestScore_ExperienceExactMatch(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.CandidateYears = 5
	input.RequiredYears = 5
	input.SeniorityMatch = "exact"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// yearsScore=1.0, seniorityScore=1.0 → (1.0*0.4 + 1.0*0.6)*25 = 25
	within(t, "ExperienceFit", 25.0, result.Breakdown.ExperienceFit, 0.1)
}

func TestScore_ExperienceOverqualified(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.CandidateYears = 15 // > 5*2.0 = 10 → overqualified
	input.RequiredYears = 5
	input.SeniorityMatch = "exact"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// yearsScore before penalty: 1.0; penalty applies → yearsScore * 0.85
	// (0.85*0.4 + 1.0*0.6)*25 = (0.34+0.6)*25 = 23.5
	within(t, "ExperienceFit", 23.5, result.Breakdown.ExperienceFit, 0.1)
}

func TestScore_ExperienceUnderqualified(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.CandidateYears = 2
	input.RequiredYears = 5
	input.SeniorityMatch = "exact"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// yearsScore = min(2/5, 1.0) = 0.4
	// (0.4*0.4 + 1.0*0.6)*25 = (0.16+0.6)*25 = 19.0
	within(t, "ExperienceFit", 19.0, result.Breakdown.ExperienceFit, 0.1)
}

func TestScore_ExperienceOneOffSeniority(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.CandidateYears = 5
	input.RequiredYears = 5
	input.SeniorityMatch = "one_off"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (1.0*0.4 + 0.8*0.6)*25 = (0.4+0.48)*25 = 22.0
	within(t, "ExperienceFit", 22.0, result.Breakdown.ExperienceFit, 0.1)
}

func TestScore_ExperienceTwoOrMoreOffSeniority(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.CandidateYears = 5
	input.RequiredYears = 5
	input.SeniorityMatch = "two_or_more_off"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// (1.0*0.4 + 0.5*0.6)*25 = (0.4+0.3)*25 = 17.5
	within(t, "ExperienceFit", 17.5, result.Breakdown.ExperienceFit, 0.1)
}

func TestScore_ExperienceZeroRequired_FullYearsScore(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.CandidateYears = 3
	input.RequiredYears = 0
	input.SeniorityMatch = "exact"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ExperienceFit", 25.0, result.Breakdown.ExperienceFit, 0.1)
}

// BUG FIX: unknown SeniorityMatch must return an error, not silently zero 60% of experience.
func TestScore_UnknownSeniorityMatch_ReturnsError(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.SeniorityMatch = "unknown_value"

	_, err := svc.Score(&input)
	if err == nil {
		t.Fatal("expected error for unknown SeniorityMatch, got nil")
	}
}

func TestScore_EmptySeniorityMatch_ReturnsError(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.SeniorityMatch = "" // zero value — never set by pipeline

	_, err := svc.Score(&input)
	if err == nil {
		t.Fatal("expected error for empty SeniorityMatch, got nil")
	}
}

// ── ImpactEvidence ────────────────────────────────────────────────────────────

func TestScore_ImpactBullets_EnoughMetrics(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = `Experience
Education
Skills
Reduced latency by 40%.
Increased revenue by $1.2M.
Cut infrastructure costs by 30%.
Improved throughput by 2x.
Deployed system for 500k users.`

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ImpactEvidence", 10.0, result.Breakdown.ImpactEvidence, 0.01)
	if len(result.MetricBullets) < 5 {
		t.Errorf("expected at least 5 metric bullets, got %d", len(result.MetricBullets))
	}
}

func TestScore_ImpactBullets_VersionNumbersNotCounted(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = `Experience
Education
Skills
Upgraded to Python 3.11.
Used Go 1.21 for the project.
Migrated to Node 18.0.`

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ImpactEvidence", 0.0, result.Breakdown.ImpactEvidence, 0.01)
}

// BUG FIX: calendar years (1900-2099) must not count as metric bullets.
func TestScore_ImpactBullets_CalendarYearsNotCounted(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = `Experience
Education
Skills
Software Engineer, Jan 2019 - Dec 2023.
Backend Developer, 2015 - 2018.
Graduated in 2014.`

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ImpactEvidence", 0.0, result.Breakdown.ImpactEvidence, 0.01)
	if len(result.MetricBullets) != 0 {
		t.Errorf("expected 0 metric bullets from date-only lines, got %d: %v",
			len(result.MetricBullets), result.MetricBullets)
	}
}

// BUG FIX: a line with both a real metric AND a version number should count —
// the version is stripped before checking, not the whole line discarded.
func TestScore_ImpactBullets_MixedMetricAndVersion(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = `Experience
Education
Skills
Reduced latency by 40% after migrating from Python 2.7 to 3.11.`

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 real metric bullet (40% survives after version strings are stripped)
	// min(1/5, 1.0) * 10 = 2.0
	within(t, "ImpactEvidence", 2.0, result.Breakdown.ImpactEvidence, 0.01)
	if len(result.MetricBullets) != 1 {
		t.Errorf("expected 1 metric bullet, got %d: %v", len(result.MetricBullets), result.MetricBullets)
	}
}

func TestScore_ImpactBullets_Mixed(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = `Experience
Education
Skills
Reduced latency by 40%.
Used Go 1.21 for the project.
Increased revenue by $1.2M.`

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2 real metric bullets (version number line excluded entirely)
	// min(2/5, 1.0)*10 = 4.0
	within(t, "ImpactEvidence", 4.0, result.Breakdown.ImpactEvidence, 0.01)
}

// ── ATSFormat ─────────────────────────────────────────────────────────────────

func TestScore_ATSFormat_AllSections(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "EXPERIENCE\nEDUCATION\nSKILLS"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ATSFormat", 10.0, result.Breakdown.ATSFormat, 0.01)
}

func TestScore_ATSFormat_WithColonSuffix(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience:\nEducation:\nSkills:"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ATSFormat", 10.0, result.Breakdown.ATSFormat, 0.01)
}

func TestScore_ATSFormat_CommonVariants(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Work Experience\nAcademic Education\nTechnical Skills"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ATSFormat", 10.0, result.Breakdown.ATSFormat, 0.01)
}

func TestScore_ATSFormat_NoSections(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "John Doe\nSoftware Engineer\nBuilt things."

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ATSFormat", 0.0, result.Breakdown.ATSFormat, 0.01)
}

// BUG FIX: body text containing "experience" / "skills" must NOT score as section headers.
func TestScore_ATSFormat_BodyTextFalsePositive(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	// These phrases contain section words but are not headers.
	input.ResumeText = "5 years of experience building distributed systems.\nStrong communication skills.\nPursued education in computer science."

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ATSFormat", 0.0, result.Breakdown.ATSFormat, 0.01)
}

func TestScore_ATSFormat_PartialSections(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nSkills\nJohn Doe built things."

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2 of 3 sections present → 2/3 * 10 ≈ 6.67
	within(t, "ATSFormat", 6.67, result.Breakdown.ATSFormat, 0.1)
}

// ── Readability ───────────────────────────────────────────────────────────────

func TestScore_Readability_NoFillerPhrases(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\nLed platform migration to cloud."

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "Readability", 10.0, result.Breakdown.Readability, 0.01)
}

func TestScore_Readability_WithFillerPhrases(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	// 3 filler phrases at 2.0 penalty each → 10 - 6 = 4
	input.ResumeText = "Experience\nEducation\nSkills\nResponsible for deploying services. Worked on CI pipelines. Helped with documentation."

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "Readability", 4.0, result.Breakdown.Readability, 0.01)
	if len(result.FillerPhrases) != 3 {
		t.Errorf("expected 3 filler phrases detected, got %d: %v", len(result.FillerPhrases), result.FillerPhrases)
	}
}

func TestScore_Readability_FloorAtZero(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	// 6 filler phrases at 2.0 each → 10 - 12 = -2, clamped to 0
	input.ResumeText = `Experience
Education
Skills
Responsible for deployments. Worked on infra. Helped with code.
Assisted in testing. Involved in design. Participated in planning.`

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "Readability", 0.0, result.Breakdown.Readability, 0.01)
}

// BUG FIX: "networked on" must not trigger "worked on" filler penalty (substring false positive).
func TestScore_Readability_NoFalsePositiveFromSubstring(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\nNetworked on-site with clients. Reworked on legacy code."

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Neither "networked on" nor "reworked on" should trigger "worked on"
	within(t, "Readability", 10.0, result.Breakdown.Readability, 0.01)
	if len(result.FillerPhrases) != 0 {
		t.Errorf("expected 0 filler phrases, got %d: %v", len(result.FillerPhrases), result.FillerPhrases)
	}
}

// ── Total & metadata ──────────────────────────────────────────────────────────

func TestScore_TotalIsSum(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := result.Breakdown.KeywordMatch + result.Breakdown.ExperienceFit +
		result.Breakdown.ImpactEvidence + result.Breakdown.ATSFormat + result.Breakdown.Readability
	within(t, "Total()", expected, result.Breakdown.Total(), 0.001)
}

func TestScore_TotalDoesNotExceed100(t *testing.T) {
	svc := scorer.New(defaults())

	// All keywords matched, exact experience, 5 metric bullets, all sections, no filler.
	input := model.ScorerInput{
		ResumeText: `Experience
Education
Skills
Reduced latency by 40%.
Increased revenue by $1.2M.
Cut costs by 30%.
Improved throughput by 2x.
Deployed system for 500k users.
Go PostgreSQL Kubernetes developer`,
		ResumeLabel:    "perfect",
		ResumePath:     "/tmp/perfect.pdf",
		JD:             model.JDData{Required: []string{"Go", "PostgreSQL"}, Preferred: []string{"Kubernetes"}},
		CandidateYears: 5,
		RequiredYears:  5,
		SeniorityMatch: "exact",
	}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Breakdown.Total() > 100.0+1e-9 {
		t.Errorf("Total() = %.4f exceeds 100", result.Breakdown.Total())
	}
}

func TestScore_MetadataPreserved(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeLabel = "my-label"
	input.ResumePath = "/path/to/resume.pdf"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ResumeLabel != "my-label" {
		t.Errorf("ResumeLabel: want %q, got %q", "my-label", result.ResumeLabel)
	}
	if result.ResumePath != "/path/to/resume.pdf" {
		t.Errorf("ResumePath: want %q, got %q", "/path/to/resume.pdf", result.ResumePath)
	}
}

// ── Error conditions ──────────────────────────────────────────────────────────

func TestScore_EmptyResumeText_ReturnsError(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = ""

	_, err := svc.Score(&input)
	if err == nil {
		t.Fatal("expected error for empty ResumeText, got nil")
	}
}

func TestScore_WhitespaceOnlyResumeText_ReturnsError(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "   \n\t  "

	_, err := svc.Score(&input)
	if err == nil {
		t.Fatal("expected error for whitespace-only ResumeText, got nil")
	}
}

func TestScore_NilInput_ReturnsError(t *testing.T) {
	svc := scorer.New(defaults())

	_, err := svc.Score(nil)
	if err == nil {
		t.Fatal("expected error for nil input, got nil")
	}
}

// ── ReferenceData passthrough ─────────────────────────────────────────────────

func TestScore_ReferenceGaps_PassedThrough(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ReferenceData = &model.ReferenceData{
		AllSkills: []string{"Go", "PostgreSQL"},
		PriorityMap: map[string]model.ReferenceGap{
			"Kubernetes": {JDSkill: "Kubernetes", RefSkill: "Docker", Priority: "high"},
		},
	}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.ReferenceGaps) != 1 {
		t.Fatalf("expected 1 reference gap, got %d", len(result.ReferenceGaps))
	}
	if result.ReferenceGaps[0].JDSkill != "Kubernetes" {
		t.Errorf("gap JDSkill: want %q, got %q", "Kubernetes", result.ReferenceGaps[0].JDSkill)
	}
}

func TestScore_ReferenceGaps_NilReferenceData(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ReferenceData = nil

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ReferenceGaps != nil {
		t.Errorf("expected nil ReferenceGaps when ReferenceData is nil, got %v", result.ReferenceGaps)
	}
}

// ── Interface compliance ──────────────────────────────────────────────────────

func TestScore_SatisfiesPortScorer(t *testing.T) {
	svc := scorer.New(defaults())
	if svc == nil {
		t.Fatal("scorer.New returned nil")
	}
}
