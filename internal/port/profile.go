package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// ProfileCompiler assembles a compiled profile from host-tagged stories and skills.
type ProfileCompiler interface {
	// Compile assembles the effective skills set (union minus removes), resolves
	// existing story IDs from the prior profile, and returns a profile with
	// orphan tracking. Returns an error if any story ID cannot be resolved.
	Compile(ctx context.Context, input model.AssembleInput) (model.CompiledProfile, error)
}

// CompiledProfileRepository persists and retrieves the compiled profile artifact.
type CompiledProfileRepository interface {
	// Load reads profile-compiled.json from dataDir.
	// Returns model.ErrProfileMissing if the file does not exist.
	// Returns model.ErrProfileSchemaMismatch if schema_version is unrecognised.
	Load(dataDir string) (model.CompiledProfile, error)

	// Save writes the profile atomically (temp file + rename) to prevent
	// partial-write corruption.
	Save(dataDir string, profile model.CompiledProfile) error

	// IsStale compares profile.CompiledAt against the mtime of every source file
	// (skills.md and accomplishments.json) in dataDir.
	// staleFiles contains basenames of files newer than CompiledAt.
	// Returns (false, nil, nil) when the profile is absent — callers MUST handle
	// model.ErrProfileMissing from Load separately to distinguish "never compiled"
	// from "compiled and current".
	IsStale(dataDir string) (stale bool, staleFiles []string, err error)

	// NeedsCompilation reports whether the host must call compile_profile.
	// Returns true when the profile is absent OR when any source file (skills.md,
	// accomplishments.json) is newer than CompiledAt. staleFiles is nil when the
	// profile is absent (there is nothing stale — the profile simply does not exist yet).
	// Prefer this over IsStale for the host-facing "should I compile?" decision;
	// IsStale is the lower-level mtime comparator that treats absence as not-stale.
	NeedsCompilation(dataDir string) (needs bool, staleFiles []string, err error)
}

// StoryCreatorService writes a new accomplishment story to disk and updates
// career.json with any new job title.
type StoryCreatorService interface {
	// Create validates input, registers the job title if is_new_job=true,
	// and writes the story to accomplishments.json.
	// Returns an error if the primary skill is not in skills.md, if required
	// SBI fields are blank, or if the job title is unknown and is_new_job=false.
	Create(ctx context.Context, input model.StoryInput) (model.StoryOutput, error)
}

// CareerRepository manages the career experience catalog used by StoryCreator
// to classify stories by job title. Stored as career.json in the data directory,
// separate from per-resume sections files to avoid two-writer conflicts.
type CareerRepository interface {
	// HasExperience reports whether a role with the given title exists in career.json.
	HasExperience(dataDir string, jobTitle string) (bool, error)

	// AppendExperience adds a new experience ref to career.json.
	// Does NOT deduplicate — callers must call HasExperience first.
	AppendExperience(dataDir string, ref model.ExperienceRef) error
}
