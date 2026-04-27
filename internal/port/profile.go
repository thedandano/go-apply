package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// ProfileCompiler tags each story with matching skills and identifies orphaned skills.
//
// Implementations MUST NOT import CompiledProfileRepository. The caller (MCP handler)
// is responsible for loading the prior profile and passing it as CompileInput.PriorProfile.
// This enforces the hexagonal architecture invariant: dependency arrows flow inward.
type ProfileCompiler interface {
	// Compile tags each story with matching skills and identifies orphaned skills.
	// On partial LLM failure, returns a CompiledProfile with PartialTaggingFailure=true
	// and affected stories with non-empty TaggingError. Never silently returns
	// zero-tagged stories on LLM failure — the error must be observable in the output.
	Compile(ctx context.Context, input model.CompileInput) (model.CompiledProfile, error)
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
	// (skills.md and accomplishments-*.md) in dataDir.
	// staleFiles contains basenames of files newer than CompiledAt.
	// Returns (false, nil, nil) when the profile is absent — callers MUST handle
	// model.ErrProfileMissing from Load separately to distinguish "never compiled"
	// from "compiled and current".
	IsStale(dataDir string) (stale bool, staleFiles []string, err error)
}

// StoryCreatorService writes a new accomplishment story to disk and updates
// career.json with any new job title.
type StoryCreatorService interface {
	// Create validates input, registers the job title if is_new_job=true,
	// and writes the story to the next available accomplishments-N.md.
	// Returns an error if the primary skill is not in skills.md, if required
	// SBI fields are blank, or if the job title is unknown and is_new_job=false.
	Create(ctx context.Context, input model.StoryInput) (model.StoryOutput, error)
}

// SectionsRepository manages the career experience catalog used by StoryCreator
// to classify stories by job title. This is separate from per-resume sections files
// and is stored as career.json in the data directory.
type SectionsRepository interface {
	// HasExperience reports whether a role with the given title exists in career.json.
	HasExperience(dataDir string, jobTitle string) (bool, error)

	// AppendExperience adds a new experience ref to career.json.
	// Does NOT deduplicate — callers must call HasExperience first.
	AppendExperience(dataDir string, ref model.ExperienceRef) error
}
