package model

// AugmentInput groups all inputs for a single augmentation pass.
type AugmentInput struct {
	ResumeText string
	RefData    *ReferenceData
	JDKeywords []string
}

// TailorSuggestion is a single profile chunk matched to a keyword.
type TailorSuggestion struct {
	Keyword    string
	SourceDoc  string
	Text       string
	Similarity float32 // 0 when from keyword-fallback path
}

// TailorSuggestions groups matched chunks by keyword.
// The map key is the keyword string.
type TailorSuggestions map[string][]TailorSuggestion
