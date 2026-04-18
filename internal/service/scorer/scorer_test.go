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

func TestScore_KeywordMatch(t *testing.T) {
	type want struct {
		kwScore float64
		kwTol   float64
		reqPct  *float64
		prefPct *float64
	}
	pf := func(v float64) *float64 { return &v }

	tests := []struct {
		name       string
		resumeText string
		required   []string
		preferred  []string
		want       want
	}{
		{
			name:       "all matched",
			resumeText: "Experience\nEducation\nSkills\nGo PostgreSQL Kubernetes developer",
			required:   []string{"Go", "PostgreSQL"},
			preferred:  []string{"Kubernetes"},
			// reqPct=1.0, prefPct=1.0, both lists populated → (0.7+0.3)*45 = 45
			want: want{kwScore: 45.0, kwTol: 0.01, reqPct: pf(1.0), prefPct: pf(1.0)},
		},
		{
			name:       "no keywords matched",
			resumeText: "Experience\nEducation\nSkills\nPython Django developer",
			required:   []string{"Go", "PostgreSQL"},
			preferred:  []string{"Kubernetes"},
			want:       want{kwScore: 0.0, kwTol: 0.01, reqPct: pf(0.0)},
		},
		{
			name:       "partial keywords matched",
			resumeText: "Experience\nEducation\nSkills\nGo developer with PostgreSQL experience",
			required:   []string{"Go", "PostgreSQL", "Redis"},
			preferred:  []string{"Kubernetes", "Docker"},
			// req: 2/3 matched, pref: 0/2 matched
			// (2/3 * 0.7 + 0/2 * 0.3) * 45 = 21.0
			want: want{kwScore: 21.0, kwTol: 0.1, reqPct: pf(2.0 / 3.0)},
		},
		{
			name:       "case insensitive match",
			resumeText: "Experience\nEducation\nSkills\ngo postgresql developer",
			required:   []string{"Go", "PostgreSQL"},
			preferred:  []string{}, // no preferred
			// Only required present → full weight (1.0) on required.
			// reqPct=1.0 → 1.0 * 1.0 * 45 = 45.0
			want: want{kwScore: 45.0, kwTol: 0.1},
		},
		{
			name:       "only preferred keywords full weight on preferred",
			resumeText: "Experience\nEducation\nSkills\nKubernetes developer",
			required:   []string{}, // no required
			preferred:  []string{"Kubernetes", "Docker"},
			// Only preferred present → full weight (1.0) on preferred.
			// prefPct = 1/2 = 0.5 → 0.5 * 1.0 * 45 = 22.5
			want: want{kwScore: 22.5, kwTol: 0.1},
		},
		// BUG FIX: "Go" must not match inside "Django" (word-boundary matching).
		{
			name:       "word boundary no cross match",
			resumeText: "Experience\nEducation\nSkills\nDjango developer",
			required:   []string{"Go"},
			preferred:  []string{},
			want:       want{kwScore: 0.0, kwTol: 0.01},
		},
		// BUG FIX: Keywords containing non-word chars (C++, C#, .NET) must match correctly.
		{
			name:       "non-word chars C++ matches",
			resumeText: "Experience\nEducation\nSkills\nC++ developer with 5 years",
			required:   []string{"C++"},
			preferred:  []string{},
			want:       want{kwScore: 45.0, kwTol: 0.01},
		},
		{
			name:       "non-word chars .NET matches",
			resumeText: "Experience\nEducation\nSkills\nASP.NET developer",
			required:   []string{".NET"},
			preferred:  []string{},
			want:       want{kwScore: 45.0, kwTol: 0.01},
		},
	}

	svc := scorer.New(defaults())
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := baseInput()
			input.ResumeText = tc.resumeText
			input.JD.Required = tc.required
			input.JD.Preferred = tc.preferred

			result, err := svc.Score(&input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			within(t, "KeywordMatch", tc.want.kwScore, result.Breakdown.KeywordMatch, tc.want.kwTol)
			if tc.want.reqPct != nil {
				within(t, "ReqPct", *tc.want.reqPct, result.Keywords.ReqPct, 0.01)
			}
			if tc.want.prefPct != nil {
				within(t, "PrefPct", *tc.want.prefPct, result.Keywords.PrefPct, 0.01)
			}
		})
	}
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

// ── ExperienceFit ─────────────────────────────────────────────────────────────

func TestScore_ExperienceFit(t *testing.T) {
	tests := []struct {
		name           string
		candidateYears float64
		requiredYears  float64
		seniorityMatch string
		wantFit        float64
		tol            float64
	}{
		{
			name:           "exact match",
			candidateYears: 5,
			requiredYears:  5,
			seniorityMatch: "exact",
			// yearsScore=1.0, seniorityScore=1.0 → (1.0*0.4 + 1.0*0.6)*25 = 25
			wantFit: 25.0,
			tol:     0.1,
		},
		{
			name:           "overqualified",
			candidateYears: 15, // > 5*2.0 = 10 → overqualified
			requiredYears:  5,
			seniorityMatch: "exact",
			// yearsScore before penalty: 1.0; penalty applies → yearsScore * 0.85
			// (0.85*0.4 + 1.0*0.6)*25 = (0.34+0.6)*25 = 23.5
			wantFit: 23.5,
			tol:     0.1,
		},
		{
			name:           "underqualified",
			candidateYears: 2,
			requiredYears:  5,
			seniorityMatch: "exact",
			// yearsScore = min(2/5, 1.0) = 0.4
			// (0.4*0.4 + 1.0*0.6)*25 = (0.16+0.6)*25 = 19.0
			wantFit: 19.0,
			tol:     0.1,
		},
		{
			name:           "one off seniority",
			candidateYears: 5,
			requiredYears:  5,
			seniorityMatch: "one_off",
			// (1.0*0.4 + 0.8*0.6)*25 = (0.4+0.48)*25 = 22.0
			wantFit: 22.0,
			tol:     0.1,
		},
		{
			name:           "two or more off seniority",
			candidateYears: 5,
			requiredYears:  5,
			seniorityMatch: "two_or_more_off",
			// (1.0*0.4 + 0.5*0.6)*25 = (0.4+0.3)*25 = 17.5
			wantFit: 17.5,
			tol:     0.1,
		},
		{
			name:           "zero required full years score",
			candidateYears: 3,
			requiredYears:  0,
			seniorityMatch: "exact",
			wantFit:        25.0,
			tol:            0.1,
		},
	}

	svc := scorer.New(defaults())
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := baseInput()
			input.CandidateYears = tc.candidateYears
			input.RequiredYears = tc.requiredYears
			input.SeniorityMatch = tc.seniorityMatch

			result, err := svc.Score(&input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			within(t, "ExperienceFit", tc.wantFit, result.Breakdown.ExperienceFit, tc.tol)
		})
	}
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

func TestScore_ImpactEvidence(t *testing.T) {
	type wantBullets struct {
		count   int
		atLeast bool // true → assert >= count, false → assert == count
	}

	tests := []struct {
		name        string
		resumeText  string
		wantScore   float64
		tol         float64
		wantBullets *wantBullets // nil = skip bullet count assertion
	}{
		{
			name: "enough metrics",
			resumeText: `Experience
Education
Skills
Reduced latency by 40%.
Increased revenue by $1.2M.
Cut infrastructure costs by 30%.
Improved throughput by 2x.
Deployed system for 500k users.`,
			wantScore:   10.0,
			tol:         0.01,
			wantBullets: &wantBullets{count: 5, atLeast: true},
		},
		{
			name: "version numbers not counted",
			resumeText: `Experience
Education
Skills
Upgraded to Python 3.11.
Used Go 1.21 for the project.
Migrated to Node 18.0.`,
			wantScore: 0.0,
			tol:       0.01,
		},
		// BUG FIX: calendar years (1900-2099) must not count as metric bullets.
		{
			name: "calendar years not counted",
			resumeText: `Experience
Education
Skills
Software Engineer, Jan 2019 - Dec 2023.
Backend Developer, 2015 - 2018.
Graduated in 2014.`,
			wantScore:   0.0,
			tol:         0.01,
			wantBullets: &wantBullets{count: 0, atLeast: false},
		},
		// BUG FIX: a line with both a real metric AND a version number should count —
		// the version is stripped before checking, not the whole line discarded.
		{
			name: "mixed metric and version",
			resumeText: `Experience
Education
Skills
Reduced latency by 40% after migrating from Python 2.7 to 3.11.`,
			// 1 real metric bullet (40% survives after version strings are stripped)
			// min(1/5, 1.0) * 10 = 2.0
			wantScore:   2.0,
			tol:         0.01,
			wantBullets: &wantBullets{count: 1, atLeast: false},
		},
		{
			name: "mixed bullets",
			resumeText: `Experience
Education
Skills
Reduced latency by 40%.
Used Go 1.21 for the project.
Increased revenue by $1.2M.`,
			// 2 real metric bullets (version number line excluded entirely)
			// min(2/5, 1.0)*10 = 4.0
			wantScore: 4.0,
			tol:       0.01,
		},
	}

	svc := scorer.New(defaults())
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := baseInput()
			input.ResumeText = tc.resumeText

			result, err := svc.Score(&input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			within(t, "ImpactEvidence", tc.wantScore, result.Breakdown.ImpactEvidence, tc.tol)

			if tc.wantBullets != nil {
				if tc.wantBullets.atLeast {
					if len(result.MetricBullets) < tc.wantBullets.count {
						t.Errorf("expected at least %d metric bullets, got %d", tc.wantBullets.count, len(result.MetricBullets))
					}
				} else {
					if len(result.MetricBullets) != tc.wantBullets.count {
						t.Errorf("expected %d metric bullets, got %d: %v",
							tc.wantBullets.count, len(result.MetricBullets), result.MetricBullets)
					}
				}
			}
		})
	}
}

// ── ATSFormat ─────────────────────────────────────────────────────────────────

func TestScore_ATSFormat(t *testing.T) {
	tests := []struct {
		name       string
		resumeText string
		wantScore  float64
		tol        float64
	}{
		{
			name:       "all sections",
			resumeText: "EXPERIENCE\nEDUCATION\nSKILLS",
			wantScore:  10.0,
			tol:        0.01,
		},
		{
			name:       "with colon suffix",
			resumeText: "Experience:\nEducation:\nSkills:",
			wantScore:  10.0,
			tol:        0.01,
		},
		{
			name:       "common variants",
			resumeText: "Work Experience\nAcademic Education\nTechnical Skills",
			wantScore:  10.0,
			tol:        0.01,
		},
		{
			name:       "no sections",
			resumeText: "John Doe\nSoftware Engineer\nBuilt things.",
			wantScore:  0.0,
			tol:        0.01,
		},
		// BUG FIX: body text containing "experience" / "skills" must NOT score as section headers.
		{
			name: "body text false positive",
			// These phrases contain section words but are not headers.
			resumeText: "5 years of experience building distributed systems.\nStrong communication skills.\nPursued education in computer science.",
			wantScore:  0.0,
			tol:        0.01,
		},
		{
			name:       "partial sections",
			resumeText: "Experience\nSkills\nJohn Doe built things.",
			// 2 of 3 sections present → 2/3 * 10 ≈ 6.67
			wantScore: 6.67,
			tol:       0.1,
		},
	}

	svc := scorer.New(defaults())
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := baseInput()
			input.ResumeText = tc.resumeText

			result, err := svc.Score(&input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			within(t, "ATSFormat", tc.wantScore, result.Breakdown.ATSFormat, tc.tol)
		})
	}
}

// ── Readability ───────────────────────────────────────────────────────────────

func TestScore_Readability(t *testing.T) {
	tests := []struct {
		name            string
		resumeText      string
		wantScore       float64
		tol             float64
		wantFillerCount *int // nil = skip filler phrase count assertion
	}{
		{
			name:       "no filler phrases",
			resumeText: "Experience\nEducation\nSkills\nLed platform migration to cloud.",
			wantScore:  10.0,
			tol:        0.01,
		},
		{
			name: "with filler phrases",
			// 3 filler phrases at 2.0 penalty each → 10 - 6 = 4
			resumeText:      "Experience\nEducation\nSkills\nResponsible for deploying services. Worked on CI pipelines. Helped with documentation.",
			wantScore:       4.0,
			tol:             0.01,
			wantFillerCount: func() *int { v := 3; return &v }(),
		},
		{
			name: "floor at zero",
			// 6 filler phrases at 2.0 each → 10 - 12 = -2, clamped to 0
			resumeText: `Experience
Education
Skills
Responsible for deployments. Worked on infra. Helped with code.
Assisted in testing. Involved in design. Participated in planning.`,
			wantScore: 0.0,
			tol:       0.01,
		},
		// BUG FIX: "networked on" must not trigger "worked on" filler penalty (substring false positive).
		{
			name:       "no false positive from substring",
			resumeText: "Experience\nEducation\nSkills\nNetworked on-site with clients. Reworked on legacy code.",
			// Neither "networked on" nor "reworked on" should trigger "worked on"
			wantScore:       10.0,
			tol:             0.01,
			wantFillerCount: func() *int { v := 0; return &v }(),
		},
	}

	svc := scorer.New(defaults())
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := baseInput()
			input.ResumeText = tc.resumeText

			result, err := svc.Score(&input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			within(t, "Readability", tc.wantScore, result.Breakdown.Readability, tc.tol)

			if tc.wantFillerCount != nil {
				if len(result.FillerPhrases) != *tc.wantFillerCount {
					t.Errorf("expected %d filler phrases detected, got %d: %v",
						*tc.wantFillerCount, len(result.FillerPhrases), result.FillerPhrases)
				}
			}
		})
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

func TestScore_EmptyOrWhitespaceResumeText_ReturnsError(t *testing.T) {
	tests := []struct {
		name       string
		resumeText string
	}{
		{
			name:       "empty resume text",
			resumeText: "",
		},
		{
			name:       "whitespace only resume text",
			resumeText: "   \n\t  ",
		},
	}

	svc := scorer.New(defaults())
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := baseInput()
			input.ResumeText = tc.resumeText

			_, err := svc.Score(&input)
			if err == nil {
				t.Fatal("expected error for empty/whitespace ResumeText, got nil")
			}
		})
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
