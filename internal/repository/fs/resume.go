// Package fs provides filesystem-backed repository implementations.
package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.ResumeRepository = (*ResumeRepository)(nil)

// ResumeRepository lists resume files from the inputs subdirectory of dataDir.
// Only .docx and .pdf files are returned; all other extensions are ignored.
type ResumeRepository struct {
	inputsDir string
}

// NewResumeRepository constructs a ResumeRepository rooted at dataDir/inputs.
func NewResumeRepository(dataDir string) *ResumeRepository {
	return &ResumeRepository{inputsDir: filepath.Join(dataDir, "inputs")}
}

// ListResumes returns all .docx and .pdf files found in the inputs directory.
func (r *ResumeRepository) ListResumes() ([]model.ResumeFile, error) {
	entries, err := os.ReadDir(r.inputsDir)
	if err != nil {
		return nil, fmt.Errorf("read inputs dir %s: %w", r.inputsDir, err)
	}
	resumes := make([]model.ResumeFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".docx" && ext != ".pdf" {
			continue
		}
		label := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		resumes = append(resumes, model.ResumeFile{
			Label: label,
			Path:  filepath.Join(r.inputsDir, e.Name()),
		})
	}
	return resumes, nil
}
