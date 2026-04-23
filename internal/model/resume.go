package model

type ResumeFile struct {
	Label    string
	Path     string
	FileType string
}

type BulletChange struct {
	Original  string `json:"original"`
	Rewritten string `json:"rewritten"`
}

type TailorResult struct {
	ResumeLabel      string         `json:"resume_label"`
	AddedKeywords    []string       `json:"added_keywords,omitempty"`
	RewrittenBullets []BulletChange `json:"rewritten_bullets,omitempty"`
	// BulletsAttempted is the number of keyword-matching bullets sent for rewriting.
	// When > 0 and RewrittenBullets is empty, every rewrite attempt failed.
	BulletsAttempted int         `json:"bullets_attempted,omitempty"`
	OutputPath       string      `json:"output_path,omitempty"`
	NewScore         ScoreResult `json:"new_score"`
	// TailoredText is the agent-produced tailored resume text.
	// Surfaced in MCP output; redacted from on-disk ApplicationRecord via its MarshalJSON.
	TailoredText string `json:"tailored_text,omitempty"`
}
