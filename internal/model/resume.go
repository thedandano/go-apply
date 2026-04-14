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
	OutputPath       string         `json:"output_path,omitempty"`
	NewScore         ScoreResult    `json:"new_score"`
	TailoredText     string         `json:"-"` // post-cascade text for accurate re-score delta; not serialized
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
