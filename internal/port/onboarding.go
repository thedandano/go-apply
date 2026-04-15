package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// Onboarder stores resume, skills, and accomplishments text into the profile
// repository, embedding each document chunk for later vector retrieval.
// Individual embed/upsert failures degrade to Warnings rather than aborting.
type Onboarder interface {
	Run(ctx context.Context, input model.OnboardInput) (model.OnboardResult, error)
}
