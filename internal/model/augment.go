package model

// AugmentInput groups all inputs for a single augmentation pass.
type AugmentInput struct {
	ResumeText string
	RefData    *ReferenceData
	JDKeywords []string
}
