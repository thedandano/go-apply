package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

type CoverLetterInput struct {
	JD      model.JDData
	Scores  map[string]model.ScoreResult
	Channel model.ChannelType
	Profile model.UserProfile
}

type CoverLetterGenerator interface {
	Generate(ctx context.Context, input *CoverLetterInput) (model.CoverLetterResult, error)
}
