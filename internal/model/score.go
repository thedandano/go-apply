package model

type ScoreBreakdown struct {
	KeywordMatch   float64 `json:"keyword_match"`
	ExperienceFit  float64 `json:"experience_fit"`
	ImpactEvidence float64 `json:"impact_evidence"`
	ATSFormat      float64 `json:"ats_format"`
	Readability    float64 `json:"readability"`
}

func (b ScoreBreakdown) Total() float64 {
	return b.KeywordMatch + b.ExperienceFit + b.ImpactEvidence + b.ATSFormat + b.Readability
}

type KeywordResult struct {
	ReqMatched    []string `json:"req_matched"`
	ReqUnmatched  []string `json:"req_unmatched"`
	PrefMatched   []string `json:"pref_matched"`
	PrefUnmatched []string `json:"pref_unmatched"`
	ReqPct        float64  `json:"req_pct"`
	PrefPct       float64  `json:"pref_pct"`
}

type ReferenceGap struct {
	JDSkill  string `json:"jd_skill"`
	RefSkill string `json:"ref_skill"`
	Priority string `json:"priority"`
	Label    string `json:"label"`
	Note     string `json:"note"`
}

type ScoreResult struct {
	ResumeLabel   string         `json:"resume_label"`
	ResumePath    string         `json:"resume_path"`
	Breakdown     ScoreBreakdown `json:"breakdown"`
	Keywords      KeywordResult  `json:"keywords"`
	MetricBullets []string       `json:"metric_bullets"`
	FillerPhrases []string       `json:"filler_phrases"`
	ReferenceGaps []ReferenceGap `json:"reference_gaps"`
}
