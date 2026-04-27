package fs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/thedandano/go-apply/internal/model"
)

func (r *ResumeRepository) sectionsPath(label string) string {
	return filepath.Join(r.inputsDir, label+".sections.json")
}

// LoadSections reads the sections JSON file for the given resume label.
// Returns model.ErrSectionsMissing if the file does not exist.
// Returns model.ErrSchemaVersionUnsupported if schema_version does not match.
func (r *ResumeRepository) LoadSections(label string) (model.SectionMap, error) {
	path := r.sectionsPath(label)
	data, err := os.ReadFile(path) // #nosec G304 -- path is data dir + sanitized label, no traversal
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return model.SectionMap{}, fmt.Errorf("load sections %s: %w", label, model.ErrSectionsMissing)
		}
		return model.SectionMap{}, fmt.Errorf("read sections file %s: %w", path, err)
	}
	var sm model.SectionMap
	if err := json.Unmarshal(data, &sm); err != nil {
		return model.SectionMap{}, fmt.Errorf("unmarshal sections file %s: %w", path, err)
	}
	if sm.SchemaVersion != model.CurrentSchemaVersion {
		return model.SectionMap{}, fmt.Errorf("schema_version %d: %w", sm.SchemaVersion, model.ErrSchemaVersionUnsupported)
	}
	if err := model.ValidateSectionMap(&sm); err != nil {
		return model.SectionMap{}, fmt.Errorf("invalid sections file %s: %w", path, err)
	}
	return sm, nil
}

// SaveSections marshals sm to JSON and writes it atomically to the sections file for label.
func (r *ResumeRepository) SaveSections(label string, sm model.SectionMap) error { //nolint:gocritic // hugeParam: interface constraint
	path := r.sectionsPath(label)
	tmp := path + ".tmp"

	data, err := json.Marshal(sm)
	if err != nil {
		return fmt.Errorf("marshal sections %s: %w", label, err)
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G306
		return fmt.Errorf("write sections file tmp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename sections file %s: %w", path, err)
	}
	return nil
}
