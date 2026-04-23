package model

type ResumeFile struct {
	Label    string
	Path     string
	FileType string
}

type TailorTier int

const (
	TierNone    TailorTier = 0
	TierKeyword TailorTier = 1
	TierBullet  TailorTier = 2
)

type BulletChange struct {
	Original  string `json:"original"`
	Rewritten string `json:"rewritten"`
}

type TailorResult struct {
	ResumeLabel      string         `json:"resume_label"`
	TierApplied      TailorTier     `json:"tier_applied"`
	AddedKeywords    []string       `json:"added_keywords,omitempty"`
	RewrittenBullets []BulletChange `json:"rewritten_bullets,omitempty"`
	// BulletsAttempted is the number of keyword-matching bullets sent to the LLM
	// during a tier-2 pass. When > 0 and RewrittenBullets is empty, every LLM call
	// failed (vs. simply no bullets matching keywords).
	BulletsAttempted int         `json:"bullets_attempted,omitempty"`
	OutputPath       string      `json:"output_path,omitempty"`
	NewScore         ScoreResult `json:"new_score"`
	// TailoredText is the latest cascade state (post-T1 or post-T2); used for
	// accurate re-score delta. Tier1Text is a T1-only snapshot preserved for
	// T1→T2 delta scoring and is always set when T1 runs.
	// Surfaced in headless + MCP output; redacted from on-disk ApplicationRecord via its MarshalJSON.
	TailoredText string       `json:"tailored_text,omitempty"`
	Tier1Text    string       `json:"tier1_text,omitempty"`  // output of tier-1 keyword injection, always set when T1 runs
	Tier1Score   *ScoreResult `json:"tier1_score,omitempty"` // score of tier-1 text; set by pipeline after TailorResume returns
}

// ResumeChanges describes the mutations the tailor service applied to a resume.
type ResumeChanges struct {
	AddedKeywords    []string
	RewrittenBullets []BulletChange
}

// TailorOptions carries behaviour-controlling limits for the tailor service.
// Values come from AppDefaults; extracted by the CLI/MCP layer before calling TailorResume.
type TailorOptions struct {
	MaxTier2BulletRewrites int
}

// TailorInput groups all inputs for a single tailor pass.
type TailorInput struct {
	Resume              ResumeFile
	ResumeText          string // pre-extracted by the pipeline before calling TailorResume
	JD                  JDData
	ScoreBefore         ScoreResult
	AccomplishmentsText string
	SkillsRefText       string
	Options             TailorOptions
}
