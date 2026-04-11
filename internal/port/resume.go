package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

type ResumeRepository interface {
	ListResumes() ([]model.ResumeFile, error)
	ExtractText(resume model.ResumeFile) (string, error)
}

type ResumeChanges struct {
	AddedKeywords    []string
	RewrittenBullets []model.BulletChange
}

// ResumeModifier generates a modified resume file via the resume modifier subprocess.
type ResumeModifier interface {
	ModifyResume(ctx context.Context, resume model.ResumeFile, changes ResumeChanges) (string, error)
}
