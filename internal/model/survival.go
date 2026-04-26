package model

// KeywordSurvival is the result of comparing JD keywords against PDF-extracted text.
type KeywordSurvival struct {
	Dropped         []string `json:"dropped"`
	Matched         []string `json:"matched"`
	TotalJDKeywords int      `json:"total_jd_keywords"`
}

var _ = KeywordSurvival{}
