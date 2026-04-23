package port

import (
	"github.com/thedandano/go-apply/internal/model"
)

// ResumeRepository lists the resume files available to the pipeline.
type ResumeRepository interface {
	ListResumes() ([]model.ResumeFile, error)
}
