package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// ResumeRepository lists the resume files available to the pipeline.
type ResumeRepository interface {
	ListResumes() ([]model.ResumeFile, error)

	// LoadSections returns the structured SectionMap for the given label.
	// Returns model.ErrSectionsMissing if the sections sidecar is absent.
	LoadSections(label string) (model.SectionMap, error)

	// SaveSections persists the SectionMap for the given label, atomically
	// replacing any prior sidecar.
	SaveSections(label string, sections model.SectionMap) error
}

// ResumeModifier generates a modified resume file by applying the given changes.
type ResumeModifier interface {
	ModifyResume(ctx context.Context, resume model.ResumeFile, changes model.ResumeChanges) (string, error)
}
