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
	OutputPath       string         `json:"output_path"`
	NewScore         ScoreResult    `json:"new_score"`
	TailoredText     string         `json:"-"` // internal: post-tailoring text for re-scoring
}
