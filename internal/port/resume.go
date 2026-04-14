package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// ResumeRepository lists the resume files available to the pipeline.
type ResumeRepository interface {
	ListResumes() ([]model.ResumeFile, error)
}

// ResumeModifier generates a modified resume file by applying the given changes.
type ResumeModifier interface {
	ModifyResume(ctx context.Context, resume model.ResumeFile, changes model.ResumeChanges) (string, error)
}
