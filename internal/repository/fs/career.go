package fs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

const careerFile = "career.json"

// CareerRepo implements port.CareerRepository using career.json in the data dir.
// mu serialises HasExperience+AppendExperience to prevent TOCTOU races on concurrent
// create_story calls with is_new_job=true.
type CareerRepo struct {
	mu sync.Mutex
}

// NewCareerRepository returns a new CareerRepo.
func NewCareerRepository() *CareerRepo { return &CareerRepo{} }

var _ port.CareerRepository = (*CareerRepo)(nil)

func careerPath(dataDir string) string { return filepath.Join(dataDir, careerFile) }

func loadCareer(dataDir string) ([]model.ExperienceRef, error) {
	data, err := os.ReadFile(careerPath(dataDir)) // #nosec G304
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read career file: %w", err)
	}
	var refs []model.ExperienceRef
	if err := json.Unmarshal(data, &refs); err != nil {
		return nil, fmt.Errorf("parse career file: %w", err)
	}
	return refs, nil
}

func saveCareer(dataDir string, refs []model.ExperienceRef) error {
	data, err := json.Marshal(refs)
	if err != nil {
		return fmt.Errorf("marshal career: %w", err)
	}
	path := careerPath(dataDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G306
		return fmt.Errorf("write career tmp: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }() // no-op after successful rename; cleans up on panic or early return
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename career file: %w", err)
	}
	return nil
}

// HasExperience reports whether a role matching jobTitle exists in career.json.
// Case-insensitive comparison.
func (r *CareerRepo) HasExperience(dataDir string, jobTitle string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	refs, err := loadCareer(dataDir)
	if err != nil {
		return false, err
	}
	needle := strings.ToLower(strings.TrimSpace(jobTitle))
	for _, ref := range refs {
		if strings.ToLower(strings.TrimSpace(ref.Role)) == needle {
			return true, nil
		}
	}
	return false, nil
}

// AppendExperience adds ref to career.json atomically.
func (r *CareerRepo) AppendExperience(dataDir string, ref model.ExperienceRef) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	refs, err := loadCareer(dataDir)
	if err != nil {
		return err
	}
	refs = append(refs, ref)
	return saveCareer(dataDir, refs)
}
