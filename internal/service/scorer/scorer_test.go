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
	d := config.EmbeddedDefaults()
	return d
}

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

	// reqPct=1.0, prefPct=1.0 → (0.7+0.3)*45 = 45
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
	// (2/3)*0.7*45 = 21.0
	within(t, "KeywordMatch", 21.0, result.Breakdown.KeywordMatch, 0.1)
	within(t, "ReqPct", 2.0/3.0, result.Keywords.ReqPct, 0.01)
}

func TestScore_KeywordMatchCaseInsensitive(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.ResumeText = "Experience\nEducation\nSkills\ngo postgresql developer"
	input.JD.Required = []string{"Go", "PostgreSQL"}
	input.JD.Preferred = []string{}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "KeywordMatch", 31.5, result.Breakdown.KeywordMatch, 0.1) // 1.0 * 0.7 * 45
}

func TestScore_NoJDKeywords_FullKeywordScore(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	input.JD.Required = []string{}
	input.JD.Preferred = []string{}

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No keywords to match → full score
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
	input.RequiredYears = 0 // no years requirement → full credit
	input.SeniorityMatch = "exact"

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	within(t, "ExperienceFit", 25.0, result.Breakdown.ExperienceFit, 0.1)
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

	// 5 metric bullets → min(5/5, 1.0)*10 = 10.0
	within(t, "ImpactEvidence", 10.0, result.Breakdown.ImpactEvidence, 0.01)
	if len(result.MetricBullets) < 5 {
		t.Errorf("expected at least 5 metric bullets, got %d", len(result.MetricBullets))
	}
}

func TestScore_ImpactBullets_VersionNumbersNotCounted(t *testing.T) {
	svc := scorer.New(defaults())

	input := baseInput()
	// Version numbers should not count as metric bullets
	input.ResumeText = `Experience
Education
Skills
Upgraded to Python 3.11.
Used Go 1.21 for the project.
Ran Windows 10 tests.
Installed Node 18.`

	result, err := svc.Score(&input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Version-number lines should not register as metric bullets
	within(t, "ImpactEvidence", 0.0, result.Breakdown.ImpactEvidence, 0.01)
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

	// 2 real metric bullets (version number excluded)
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

// ── Interface compliance ──────────────────────────────────────────────────────

func TestScore_SatisfiesPortScorer(t *testing.T) {
	svc := scorer.New(defaults())
	_ = svc // compile-time check is in scorer.go; this verifies New() returns usable type
	if svc == nil {
		t.Fatal("scorer.New returned nil")
	}
}
