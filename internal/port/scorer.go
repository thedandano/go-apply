package port

import "github.com/thedandano/go-apply/internal/model"

type ReferenceData struct {
	AllSkills   []string
	PriorityMap map[string]model.ReferenceGap
}

type ScorerInput struct {
	ResumeText     string
	ResumeLabel    string
	ResumePath     string
	JD             model.JDData
	CandidateYears float64
	RequiredYears  float64
	SeniorityMatch string
	ReferenceData  *ReferenceData
}

type Scorer interface {
	Score(input ScorerInput) (model.ScoreResult, error)
}
