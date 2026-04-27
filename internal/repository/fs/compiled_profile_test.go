package fs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/repository/fs"
)

func newProfileRepo() *fs.CompiledProfileRepo {
	return fs.NewCompiledProfileRepository()
}

// TestLoad_ProfileMissing verifies ErrProfileMissing when file absent.
func TestLoad_ProfileMissing(t *testing.T) {
	dir := t.TempDir()
	repo := newProfileRepo()
	_, err := repo.Load(dir)
	if !errors.Is(err, model.ErrProfileMissing) {
		t.Fatalf("expected ErrProfileMissing, got %v", err)
	}
}

// TestLoad_SchemaMismatch verifies ErrProfileSchemaMismatch on unknown schema_version.
func TestLoad_SchemaMismatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "profile-compiled.json"), `{"schema_version":"99","compiled_at":"2026-01-01T00:00:00Z","stories":[],"orphaned_skills":[],"partial_tagging_failure":false}`)
	repo := newProfileRepo()
	_, err := repo.Load(dir)
	if !errors.Is(err, model.ErrProfileSchemaMismatch) {
		t.Fatalf("expected ErrProfileSchemaMismatch, got %v", err)
	}
}

// TestSave_Succeeds verifies the profile is written and parseable.
func TestSave_Succeeds(t *testing.T) {
	dir := t.TempDir()
	repo := newProfileRepo()
	profile := model.CompiledProfile{
		SchemaVersion:  "1",
		CompiledAt:     time.Now().UTC().Truncate(time.Second),
		Stories:        []model.Story{},
		OrphanedSkills: []model.OrphanedSkill{},
	}
	if err := repo.Save(dir, profile); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := repo.Load(dir)
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.SchemaVersion != "1" {
		t.Errorf("schema_version = %q, want 1", loaded.SchemaVersion)
	}
}

// TestSave_Atomic verifies no partial write on failure.
// We simulate this by pre-writing a good profile and verifying it survives
// a Save that fails at the rename step.
func TestSave_Atomic(t *testing.T) {
	dir := t.TempDir()
	repo := newProfileRepo()
	// Write an initial good profile.
	initial := model.CompiledProfile{
		SchemaVersion:  "1",
		CompiledAt:     time.Now().UTC(),
		Stories:        []model.Story{},
		OrphanedSkills: []model.OrphanedSkill{},
	}
	if err := repo.Save(dir, initial); err != nil {
		t.Fatalf("initial Save: %v", err)
	}

	// Make the destination directory read-only to force rename failure.
	profilePath := filepath.Join(dir, "profile-compiled.json")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip("cannot make dir read-only:", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	bad := model.CompiledProfile{SchemaVersion: "1", CompiledAt: time.Now().UTC()}
	_ = repo.Save(dir, bad) // expected to fail; ignore error

	// The original profile must still be intact.
	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("profile missing after failed Save: %v", err)
	}
	if len(data) == 0 {
		t.Error("profile file empty after failed Save")
	}
}

// TestIsStale_ProfileAbsent verifies IsStale returns (false,nil,nil) when no profile.
func TestIsStale_ProfileAbsent(t *testing.T) {
	dir := t.TempDir()
	repo := newProfileRepo()
	stale, files, err := repo.IsStale(dir)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if stale {
		t.Error("stale=true for absent profile; want false")
	}
	if len(files) != 0 {
		t.Errorf("stale_files=%v for absent profile; want empty", files)
	}
}

// TestIsStale_FileNewerThanProfile verifies stale detection.
func TestIsStale_FileNewerThanProfile(t *testing.T) {
	dir := t.TempDir()
	repo := newProfileRepo()

	// Write a profile compiled "in the past".
	past := time.Now().Add(-10 * time.Minute).UTC()
	profile := model.CompiledProfile{
		SchemaVersion:  "1",
		CompiledAt:     past,
		Stories:        []model.Story{},
		OrphanedSkills: []model.OrphanedSkill{},
	}
	if err := repo.Save(dir, profile); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Write an accomplishments file and set its mtime to "now" (newer than compiledAt).
	accPath := filepath.Join(dir, "accomplishments-2.md")
	writeFile(t, accPath, "## story")
	future := time.Now().Add(time.Minute)
	if err := os.Chtimes(accPath, future, future); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	stale, files, err := repo.IsStale(dir)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if !stale {
		t.Error("stale=false; want true")
	}
	if len(files) != 1 || files[0] != "accomplishments-2.md" {
		t.Errorf("stale_files=%v; want [accomplishments-2.md]", files)
	}
}

// TestIsStale_AllSourcesOlderThanProfile verifies no stale when profile is fresh.
func TestIsStale_AllSourcesOlderThanProfile(t *testing.T) {
	dir := t.TempDir()
	repo := newProfileRepo()

	// Write source files first with old mtime.
	accPath := filepath.Join(dir, "accomplishments-0.md")
	writeFile(t, accPath, "## story")
	old := time.Now().Add(-20 * time.Minute)
	if err := os.Chtimes(accPath, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	// Profile compiled after source files.
	profile := model.CompiledProfile{
		SchemaVersion:  "1",
		CompiledAt:     time.Now().Add(-5 * time.Minute).UTC(),
		Stories:        []model.Story{},
		OrphanedSkills: []model.OrphanedSkill{},
	}
	if err := repo.Save(dir, profile); err != nil {
		t.Fatalf("Save: %v", err)
	}

	stale, files, err := repo.IsStale(dir)
	if err != nil {
		t.Fatalf("IsStale: %v", err)
	}
	if stale {
		t.Errorf("stale=true; want false; stale_files=%v", files)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeFile %s: %v", path, err)
	}
}
