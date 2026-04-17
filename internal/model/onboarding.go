package model

// ResumeEntry pairs a text-extracted resume with its label.
type ResumeEntry struct {
	Label string
	Text  string
}

// OnboardInput groups all inputs for a single onboarding pass.
type OnboardInput struct {
	Resumes             []ResumeEntry
	SkillsText          string
	AccomplishmentsText string
}

// OnboardSummary provides a breakdown of what was embedded during onboarding.
type OnboardSummary struct {
	ResumesAdded         int `json:"resumes_added"`
	SkillsCount          int `json:"skills_count"`
	AccomplishmentsCount int `json:"accomplishments_count"`
	TotalChunks          int `json:"total_chunks"`
}

// OnboardResult reports what was stored and any non-fatal failures.
type OnboardResult struct {
	Stored   []string       `json:"stored"`
	Warnings []RiskWarning  `json:"warnings,omitempty"`
	Summary  OnboardSummary `json:"summary"`
}
