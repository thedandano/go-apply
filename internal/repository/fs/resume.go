package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

var _ port.ResumeRepository = (*ResumeRepository)(nil)

// allowedExts is the set of file extensions that ListResumes returns.
var allowedExts = map[string]bool{
	".pdf":  true,
	".docx": true,
	".txt":  true,
}

// ResumeRepository lists resume files from a local directory.
type ResumeRepository struct {
	dir string
}

// NewResumeRepository creates a ResumeRepository that reads from dir.
func NewResumeRepository(dir string) *ResumeRepository {
	return &ResumeRepository{dir: dir}
}

// ListResumes returns all .pdf, .docx, and .txt files in the repository directory.
// It does not recurse into subdirectories.
// An empty directory returns an empty (non-nil) slice.
func (r *ResumeRepository) ListResumes() ([]model.ResumeFile, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("list resumes in %s: %w", r.dir, err)
	}

	results := make([]model.ResumeFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if !allowedExts[ext] {
			continue
		}
		label := strings.TrimSuffix(name, filepath.Ext(name))
		absPath := filepath.Join(r.dir, name)
		results = append(results, model.ResumeFile{
			Label:    label,
			Path:     absPath,
			FileType: ext,
		})
	}
	return results, nil
}
