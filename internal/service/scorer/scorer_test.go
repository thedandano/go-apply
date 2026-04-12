package scorer_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

func mustDefaults(t *testing.T) *config.AppDefaults {
	t.Helper()
	d, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}
	return d
}

func TestScore_KeywordMatch_PerfectMatch(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	input := port.ScorerInput{
		ResumeText:     "Experienced in Go and Kubernetes. Docker containerization.",
		JD:             model.JDData{Required: []string{"Go", "Kubernetes"}, Preferred: []string{"Docker"}},
		CandidateYears: 5,
		RequiredYears:  5,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// req_pct=100%, pref_pct=100% → min(45*1+45*0.3*1, 45) = 45.0
	if result.Breakdown.KeywordMatch != 45.0 {
		t.Errorf("KeywordMatch: want 45.0, got %.1f", result.Breakdown.KeywordMatch)
	}
	if result.Keywords.ReqPct != 100.0 {
		t.Errorf("ReqPct: want 100.0, got %.1f", result.Keywords.ReqPct)
	}
	if result.Keywords.PrefPct != 100.0 {
		t.Errorf("PrefPct: want 100.0, got %.1f", result.Keywords.PrefPct)
	}
	if len(result.Keywords.ReqUnmatched) != 0 {
		t.Errorf("ReqUnmatched: want [], got %v", result.Keywords.ReqUnmatched)
	}
}

func TestScore_KeywordMatch_NoMatch(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	input := port.ScorerInput{
		ResumeText:     "Java developer with Spring Boot expertise",
		JD:             model.JDData{Required: []string{"Python"}, Preferred: []string{}},
		CandidateYears: 3,
		RequiredYears:  3,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result.Breakdown.KeywordMatch != 0.0 {
		t.Errorf("KeywordMatch: want 0.0, got %.1f", result.Breakdown.KeywordMatch)
	}
	if len(result.Keywords.ReqUnmatched) != 1 || result.Keywords.ReqUnmatched[0] != "Python" {
		t.Errorf("ReqUnmatched: want [Python], got %v", result.Keywords.ReqUnmatched)
	}
}

func TestScore_KeywordMatch_AbbreviationExpansion(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	// "k8s" in resume should match "Kubernetes" in JD
	input := port.ScorerInput{
		ResumeText:     "Managed k8s clusters in production.",
		JD:             model.JDData{Required: []string{"Kubernetes"}, Preferred: []string{}},
		CandidateYears: 4,
		RequiredYears:  4,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result.Breakdown.KeywordMatch != 45.0 {
		t.Errorf("expected abbreviation expansion to match k8s→Kubernetes, got %.1f", result.Breakdown.KeywordMatch)
	}
}

func TestScore_KeywordMatch_NoPreferred_OnlyRequired(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	// No preferred skills → score = 45 * req_pct
	input := port.ScorerInput{
		ResumeText:     "Python developer with Django",
		JD:             model.JDData{Required: []string{"Python", "Django", "Flask"}, Preferred: []string{}},
		CandidateYears: 3,
		RequiredYears:  3,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// matched: Python, Django; unmatched: Flask → 2/3 req_pct → 45 * (2/3) = 30.0
	want := 30.0
	if result.Breakdown.KeywordMatch != want {
		t.Errorf("KeywordMatch: want %.1f, got %.1f", want, result.Breakdown.KeywordMatch)
	}
}

func TestScore_ExperienceFit_ExactSeniority(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	input := port.ScorerInput{
		ResumeText:     "five years experience",
		JD:             model.JDData{Required: []string{}},
		CandidateYears: 5.0,
		RequiredYears:  5.0,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// years_ratio=1.0, seniority_mult=1.0, no overqualification
	// 25 * (0.6*1.0 + 0.4*1.0) = 25.0
	if result.Breakdown.ExperienceFit != 25.0 {
		t.Errorf("ExperienceFit: want 25.0, got %.1f", result.Breakdown.ExperienceFit)
	}
}

func TestScore_ExperienceFit_Overqualified(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	// candidate_years > 2 * required_years → 0.85 penalty
	input := port.ScorerInput{
		ResumeText:     "senior engineer",
		JD:             model.JDData{Required: []string{}},
		CandidateYears: 12.0,
		RequiredYears:  5.0,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// years_ratio=min(12/5,1)=1.0, seniority_mult=1.0 → raw=25.0 → 25.0*0.85 = 21.25 → rounds to 21.2
	// Python: round(21.25, 1) = 21.2 (banker's rounding) — need to match exactly
	wantLo, wantHi := 21.0, 21.3
	got := result.Breakdown.ExperienceFit
	if got < wantLo || got > wantHi {
		t.Errorf("ExperienceFit (overqualified): want ~21.2, got %.2f", got)
	}
}

func TestScore_ExperienceFit_OneOff(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	input := port.ScorerInput{
		ResumeText:     "engineer",
		JD:             model.JDData{Required: []string{}},
		CandidateYears: 5.0,
		RequiredYears:  5.0,
		SeniorityMatch: "one_off",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// years_ratio=1.0, seniority_mult=0.8
	// 25 * (0.6*0.8 + 0.4*1.0) = 25 * (0.48 + 0.40) = 25 * 0.88 = 22.0
	if result.Breakdown.ExperienceFit != 22.0 {
		t.Errorf("ExperienceFit (one_off): want 22.0, got %.1f", result.Breakdown.ExperienceFit)
	}
}

func TestScore_ExperienceFit_ZeroRequiredYears(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	input := port.ScorerInput{
		ResumeText:     "engineer",
		JD:             model.JDData{Required: []string{}},
		CandidateYears: 3.0,
		RequiredYears:  0.0,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// required_years==0 → years_ratio=1.0, raw=25.0
	// BUT: candidate_years(3) > 2*required_years(0) = 0 → overqualification fires → 25.0 * 0.85 ≈ 21.2
	// Python round(21.25, 1) = 21.2; Go math.Round gives 21.3 (half-up vs banker's)
	// Accept either value — the difference is a 0.1pt rounding artefact.
	got := result.Breakdown.ExperienceFit
	if got < 21.0 || got > 21.5 {
		t.Errorf("ExperienceFit (zero required+overqual): want ~21.2–21.3, got %.2f", got)
	}
}

func TestScore_ImpactEvidence_MetricBullets(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	// All 5 bullets use metric patterns from score.py:
	//   30%       → \d+\s*%
	//   50ms      → \d+\s*(ms|...)
	//   15 engineers → \d+...engineers
	//   10k transactions → \d+[kmb]\b
	//   $200k     → \$\s*\d
	resume := `Experience
- Increased revenue by 30%
- Reduced latency by 50ms
- Led a team of 15 engineers
- Processed 10k transactions daily
- Cut costs by $200k
`
	input := port.ScorerInput{
		ResumeText:     resume,
		JD:             model.JDData{Required: []string{}},
		CandidateYears: 5,
		RequiredYears:  5,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// 5 metric bullets → 10 * min(5/5, 1.0) = 10.0
	if result.Breakdown.ImpactEvidence != 10.0 {
		t.Errorf("ImpactEvidence: want 10.0, got %.1f", result.Breakdown.ImpactEvidence)
	}
	if len(result.MetricBullets) != 5 {
		t.Errorf("MetricBullets: want 5, got %d: %v", len(result.MetricBullets), result.MetricBullets)
	}
}

func TestScore_ImpactEvidence_ThreeBullets(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	// 3 bullets that all match metric patterns:
	//   20%         → \d+\s*%
	//   150ms       → \d+\s*(ms|...)
	//   8 members   → \d+...members
	resume := `Experience
- Increased sales by 20%
- Reduced p99 latency from 300ms to 150ms
- Managed 8 team members
`
	input := port.ScorerInput{
		ResumeText:     resume,
		JD:             model.JDData{Required: []string{}},
		CandidateYears: 4,
		RequiredYears:  4,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	// 3 metric bullets → 10 * min(3/5, 1.0) = 6.0
	if result.Breakdown.ImpactEvidence != 6.0 {
		t.Errorf("ImpactEvidence: want 6.0, got %.1f", result.Breakdown.ImpactEvidence)
	}
}

func TestScore_FillerPhrases(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	resume := "Responsible for backend services. Assisted with deployment. Worked on frontend."
	input := port.ScorerInput{
		ResumeText:     resume,
		JD:             model.JDData{Required: []string{}},
		CandidateYears: 3,
		RequiredYears:  3,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(result.FillerPhrases) != 3 {
		t.Errorf("FillerPhrases: want 3 (responsible for, assisted with, worked on), got %d: %v",
			len(result.FillerPhrases), result.FillerPhrases)
	}
}

func TestScore_ResumeLabel_ResumePath_Propagated(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	input := port.ScorerInput{
		ResumeLabel:    "backend",
		ResumePath:     "/resumes/backend.docx",
		ResumeText:     "Go developer",
		JD:             model.JDData{Required: []string{}},
		CandidateYears: 3,
		RequiredYears:  3,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if result.ResumeLabel != "backend" {
		t.Errorf("ResumeLabel: want backend, got %q", result.ResumeLabel)
	}
	if result.ResumePath != "/resumes/backend.docx" {
		t.Errorf("ResumePath: want /resumes/backend.docx, got %q", result.ResumePath)
	}
}

func TestScore_TotalBreakdown_SumsCorrectly(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	input := port.ScorerInput{
		ResumeText:     "Go developer with Kubernetes experience. Increased sales by 20%.",
		JD:             model.JDData{Required: []string{"Go"}, Preferred: []string{"Kubernetes"}},
		CandidateYears: 5,
		RequiredYears:  5,
		SeniorityMatch: "exact",
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	total := result.Breakdown.Total()
	if total < 0 || total > 100 {
		t.Errorf("total score out of range [0,100]: %.1f", total)
	}
	if result.Breakdown.KeywordMatch < 0 || result.Breakdown.KeywordMatch > 45 {
		t.Errorf("KeywordMatch out of range [0,45]: %.1f", result.Breakdown.KeywordMatch)
	}
	if result.Breakdown.ExperienceFit < 0 || result.Breakdown.ExperienceFit > 25 {
		t.Errorf("ExperienceFit out of range [0,25]: %.1f", result.Breakdown.ExperienceFit)
	}
	if result.Breakdown.ImpactEvidence < 0 || result.Breakdown.ImpactEvidence > 10 {
		t.Errorf("ImpactEvidence out of range [0,10]: %.1f", result.Breakdown.ImpactEvidence)
	}
	if result.Breakdown.ATSFormat < 0 || result.Breakdown.ATSFormat > 10 {
		t.Errorf("ATSFormat out of range [0,10]: %.1f", result.Breakdown.ATSFormat)
	}
	if result.Breakdown.Readability < 0 || result.Breakdown.Readability > 10 {
		t.Errorf("Readability out of range [0,10]: %.1f", result.Breakdown.Readability)
	}
}

func TestScore_ReferenceData_GapCrossReference(t *testing.T) {
	svc := scorer.New(mustDefaults(t))
	// JD requires "Kubernetes" but resume only has Docker
	// RefData has Kubernetes as a known skill
	input := port.ScorerInput{
		ResumeText:     "Docker container orchestration",
		JD:             model.JDData{Required: []string{"Kubernetes"}, Preferred: []string{}},
		CandidateYears: 4,
		RequiredYears:  4,
		SeniorityMatch: "exact",
		ReferenceData: &port.ReferenceData{
			AllSkills: []string{"Kubernetes", "Docker", "Helm"},
			PriorityMap: map[string]model.ReferenceGap{
				"Kubernetes": {RefSkill: "Kubernetes", Priority: "P0", Label: "critical"},
			},
		},
	}
	result, err := svc.Score(input)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if len(result.ReferenceGaps) == 0 {
		t.Error("expected ReferenceGaps to contain Kubernetes gap, got none")
	}
	if result.ReferenceGaps[0].JDSkill != "Kubernetes" {
		t.Errorf("ReferenceGaps[0].JDSkill: want Kubernetes, got %q", result.ReferenceGaps[0].JDSkill)
	}
}
