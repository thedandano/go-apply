package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

type ScoringWeights struct {
	KeywordMatch   float64 `json:"keyword_match"`
	ExperienceFit  float64 `json:"experience_fit"`
	ImpactEvidence float64 `json:"impact_evidence"`
	ATSFormat      float64 `json:"ats_format"`
	Readability    float64 `json:"readability"`
}

type ScoringDefaults struct {
	Weights                        ScoringWeights     `json:"weights"`
	KeywordRequiredWeight          float64            `json:"keyword_required_weight"`
	KeywordPreferredWeight         float64            `json:"keyword_preferred_weight"`
	ExperienceSeniorityWeight      float64            `json:"experience_seniority_weight"`
	ExperienceYearsWeight          float64            `json:"experience_years_weight"`
	SeniorityMultipliers           map[string]float64 `json:"seniority_multipliers"`
	OverqualificationThresholdMult float64            `json:"overqualification_threshold_multiplier"`
	OverqualificationPenalty       float64            `json:"overqualification_penalty"`
	ImpactBulletTarget             int                `json:"impact_bullet_target"`
	FillerPhrases                  []string           `json:"filler_phrases"`
	ReadabilityPenaltyPerFiller    float64            `json:"readability_penalty_per_filler"`
}

type ThresholdDefaults struct {
	ScorePass          float64 `json:"score_pass"`
	ScoreBoostMin      float64 `json:"score_boost_min"`
	MaxBoostIterations int     `json:"max_boost_iterations"`
}

type CoverLetterDefaults struct {
	MaxWords      int `json:"max_words"`
	SentenceCount int `json:"sentence_count"`
	TargetWords   int `json:"target_words"`
}

type TailorDefaults struct {
	MaxTier2BulletRewrites          int     `json:"max_tier2_bullet_rewrites"`
	MinBlendDelta                   float64 `json:"min_blend_delta"`
	KeywordRelevanceRequiredWeight  float64 `json:"keyword_relevance_required_weight"`
	KeywordRelevancePreferredWeight float64 `json:"keyword_relevance_preferred_weight"`
	BulletRelevanceKeywordWeight    float64 `json:"bullet_relevance_keyword_weight"`
	BulletRelevanceMetricWeight     float64 `json:"bullet_relevance_metric_weight"`
}

type FetcherDefaults struct {
	ChromedpTimeoutMS    int `json:"chromedp_timeout_ms"`
	MinJDTextLengthChars int `json:"min_jd_text_length_chars"`
	MaxJDTextLengthChars int `json:"max_jd_text_length_chars"`
}

type VectorSearchDefaults struct {
	SimilarityThreshold float64 `json:"similarity_threshold"`
	TopK                int     `json:"top_k"`
	DefaultEmbeddingDim int     `json:"default_embedding_dim"`
}

type LLMDefaults struct {
	TimeoutMS                  int     `json:"timeout_ms"`
	KeywordExtractionTemp      float64 `json:"keyword_extraction_temp"`
	KeywordExtractionMaxTokens int     `json:"keyword_extraction_max_tokens"`
	CoverLetterTemp            float64 `json:"cover_letter_temp"`
	CoverLetterMaxTokens       int     `json:"cover_letter_max_tokens"`
	BulletRewriteTemp          float64 `json:"bullet_rewrite_temp"`
	BulletRewriteMaxTokens     int     `json:"bullet_rewrite_max_tokens"`
}

type AugmentDefaults struct {
	IncorporationTemp      float64 `json:"incorporation_temp"`
	IncorporationMaxTokens int     `json:"incorporation_max_tokens"`
	KeywordMatchMinCount   int     `json:"keyword_match_min_count"`
	MaxChunksToIncorporate int     `json:"max_chunks_to_incorporate"`
}

// AppDefaults holds all tunable constants loaded from internal/config/defaults.json.
// Injected into services — never read inline constants from source code.
type AppDefaults struct {
	Scoring      ScoringDefaults      `json:"scoring"`
	Thresholds   ThresholdDefaults    `json:"thresholds"`
	CoverLetter  CoverLetterDefaults  `json:"cover_letter"`
	Tailor       TailorDefaults       `json:"tailor"`
	Fetcher      FetcherDefaults      `json:"fetcher"`
	VectorSearch VectorSearchDefaults `json:"vector_search"`
	LLM          LLMDefaults          `json:"llm"`
	Augment      AugmentDefaults      `json:"augment"`
}

// defaultsJSON is the embedded defaults.json.
// Eliminates runtime file lookup — works in installed binaries.
//
//go:embed defaults.json
var defaultsJSON []byte

// LoadDefaults reads defaults from the embedded defaults.json.
func LoadDefaults() (*AppDefaults, error) {
	var d AppDefaults
	if err := json.Unmarshal(defaultsJSON, &d); err != nil {
		return nil, fmt.Errorf("parse embedded defaults.json: %w", err)
	}
	return &d, nil
}

// EmbeddedDefaults loads defaults from the embedded defaults.json.
// defaults.json is the single source of truth — no values are duplicated here.
// Panics if the embedded JSON is malformed (compile-time guarantees this cannot happen).
func EmbeddedDefaults() *AppDefaults {
	d, err := LoadDefaults()
	if err != nil {
		panic("config: embedded defaults.json is malformed: " + err.Error())
	}
	return d
}
