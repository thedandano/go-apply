package port

import "github.com/thedandano/go-apply/internal/model"

// Scorer computes a numeric score for a resume against a job description.
type Scorer interface {
	Score(input *model.ScorerInput) (model.ScoreResult, error)
}
