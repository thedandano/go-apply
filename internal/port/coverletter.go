package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// CoverLetterGenerator produces a cover letter from job and candidate context.
type CoverLetterGenerator interface {
	Generate(ctx context.Context, input *model.CoverLetterInput) (model.CoverLetterResult, error)
}
