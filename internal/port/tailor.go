package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// Tailor rewrites a resume to better match a job description.
// The pipeline drives the tier loop; TailorResume executes a single tier pass.
type Tailor interface {
	TailorResume(ctx context.Context, input model.TailorInput) (model.TailorResult, error)
}
