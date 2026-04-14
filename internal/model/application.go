package model

// ApplicationOutcome tracks the lifecycle state of a job application.
type ApplicationOutcome string

const (
	OutcomePending   ApplicationOutcome = "pending"
	OutcomeInterview ApplicationOutcome = "interview"
	OutcomeOffer     ApplicationOutcome = "offer"
	OutcomeRejected  ApplicationOutcome = "rejected"
	OutcomeWithdrawn ApplicationOutcome = "withdrawn"
)

// ApplicationRecord is the persistent artifact for a single job URL.
// It is written progressively: the cache layer sets URL+RawText+JD first;
// Score, TailorResult, and CoverLetter are added as each pipeline stage runs;
// submission metadata is set by the user at apply time.
//
// This is the source of truth for batch rescoring — RawText is kept so old
// listings can be re-processed without re-fetching.
type ApplicationRecord struct {
	// Cache identity — always populated on first fetch.
	URL     string `json:"url"`
	RawText string `json:"raw_text"` // fetched page text; kept for rescoring without re-fetching

	// Extracted JD — populated when the LLM parses the raw text.
	JD JDData `json:"jd"`

	// Pipeline outputs — populated progressively; omitted until each stage runs.
	Score        *ScoreResult  `json:"score,omitempty"`
	TailorResult *TailorResult `json:"tailor_result,omitempty"`
	CoverLetter  string        `json:"cover_letter,omitempty"`

	// Submission metadata — set by the user at apply time via CLI or MCP.
	Applied     string             `json:"applied,omitempty"`      // ISO date: "2026-03-20"
	Channel     string             `json:"channel,omitempty"`      // "referral", "linkedin", etc.
	ResumeLabel string             `json:"resume_label,omitempty"` // which resume was submitted
	ResumeText  string             `json:"resume_text,omitempty"`  // snapshot of resume at submission
	Outcome     ApplicationOutcome `json:"outcome,omitempty"`
}
