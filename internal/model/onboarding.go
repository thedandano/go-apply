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

// OnboardResult reports what was stored and any non-fatal failures.
type OnboardResult struct {
	Stored   []string      `json:"stored"`
	Warnings []RiskWarning `json:"warnings,omitempty"`
}
