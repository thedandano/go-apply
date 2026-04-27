package fs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

const (
	compiledProfileFile = "profile-compiled.json"
	profileSchemaV1     = "1"
)

// CompiledProfileRepo implements port.CompiledProfileRepository.
type CompiledProfileRepo struct{}

// NewCompiledProfileRepository returns a new CompiledProfileRepo.
func NewCompiledProfileRepository() *CompiledProfileRepo {
	return &CompiledProfileRepo{}
}

var _ port.CompiledProfileRepository = (*CompiledProfileRepo)(nil)

// Load reads profile-compiled.json from dataDir.
// Returns model.ErrProfileMissing if the file does not exist.
// Returns model.ErrProfileSchemaMismatch if schema_version is unrecognised.
func (r *CompiledProfileRepo) Load(dataDir string) (model.CompiledProfile, error) {
	path := filepath.Join(dataDir, compiledProfileFile)
	data, err := os.ReadFile(path) // #nosec G304 -- dataDir is a trusted config value
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return model.CompiledProfile{}, model.ErrProfileMissing
		}
		return model.CompiledProfile{}, fmt.Errorf("read compiled profile: %w", err)
	}

	var p model.CompiledProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return model.CompiledProfile{}, fmt.Errorf("parse compiled profile: %w", err)
	}
	if p.SchemaVersion != profileSchemaV1 {
		return model.CompiledProfile{}, fmt.Errorf("schema_version %q: %w", p.SchemaVersion, model.ErrProfileSchemaMismatch)
	}
	return p, nil
}

// Save writes profile to profile-compiled.json atomically using a temp file + rename.
func (r *CompiledProfileRepo) Save(dataDir string, profile model.CompiledProfile) error { //nolint:gocritic // hugeParam: CompiledProfile is 96B — interface signature fixed
	if profile.SchemaVersion == "" {
		profile.SchemaVersion = profileSchemaV1
	}
	data, err := json.Marshal(profile)
	if err != nil {
		return fmt.Errorf("marshal compiled profile: %w", err)
	}

	path := filepath.Join(dataDir, compiledProfileFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G306
		return fmt.Errorf("write compiled profile tmp: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }() // no-op after successful rename; cleans up on panic or early return
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename compiled profile: %w", err)
	}
	return nil
}

// IsStale compares profile.CompiledAt against the mtime of every source file in dataDir.
// Returns (false, nil, nil) when the profile is absent — callers must handle ErrProfileMissing
// from Load separately to distinguish "never compiled" from "compiled and current".
func (r *CompiledProfileRepo) IsStale(dataDir string) (bool, []string, error) {
	profilePath := filepath.Join(dataDir, compiledProfileFile)
	profileData, err := os.ReadFile(profilePath) // #nosec G304
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("read profile for staleness check: %w", err)
	}

	var p model.CompiledProfile
	if err := json.Unmarshal(profileData, &p); err != nil {
		return false, nil, fmt.Errorf("parse profile for staleness check: %w", err)
	}

	compiledAt := p.CompiledAt

	// Collect source files: skills.md + accomplishments-*.md
	matches, _ := filepath.Glob(filepath.Join(dataDir, "accomplishments-*.md"))
	sources := make([]string, 0, 1+len(matches))
	if _, err := os.Stat(filepath.Join(dataDir, "skills.md")); err == nil {
		sources = append(sources, "skills.md")
	}
	for _, m := range matches {
		sources = append(sources, filepath.Base(m))
	}

	var staleFiles []string
	for _, name := range sources {
		info, statErr := os.Stat(filepath.Join(dataDir, name)) // #nosec G304
		if statErr != nil {
			slog.Warn("compiled_profile: skipping unreadable source file in staleness check",
				slog.String("file", name),
				slog.String("error", statErr.Error()),
			)
			continue
		}
		if info.ModTime().After(compiledAt) {
			staleFiles = append(staleFiles, name)
		}
	}
	return len(staleFiles) > 0, staleFiles, nil
}
