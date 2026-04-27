package model

import "time"

// StoryType classifies an accomplishment story for tailoring match.
type StoryType string

const (
	StoryTypeProject       StoryType = "project"
	StoryTypeAchievement   StoryType = "achievement"
	StoryTypeTechnical     StoryType = "technical"
	StoryTypeLeadership    StoryType = "leadership"
	StoryTypeProcess       StoryType = "process"
	StoryTypeCollaboration StoryType = "collaboration"
)

// Story is a canonical accomplishment entry with skill tags and classification.
// Source content lives in accomplishments-N.md; the compiled copy is stored here
// for fast access during tailoring.
type Story struct {
	ID           string    `json:"id"`
	SourceFile   string    `json:"source_file"`
	Text         string    `json:"text"`
	Skills       []string  `json:"skills"`
	Format       string    `json:"format"`
	Type         StoryType `json:"type"`
	JobTitle     string    `json:"job_title"`
	TaggingError string    `json:"tagging_error"`
}

// OrphanedSkill is a skill label with no successfully-tagged supporting story.
type OrphanedSkill struct {
	Skill    string `json:"skill"`
	Deferred bool   `json:"deferred"`
}

// CompiledProfile is the derived artifact produced by compilation.
// Written to ~/.local/share/go-apply/profile-compiled.json. Never hand-edited.
type CompiledProfile struct {
	SchemaVersion         string          `json:"schema_version"`
	CompiledAt            time.Time       `json:"compiled_at"`
	Stories               []Story         `json:"stories"`
	OrphanedSkills        []OrphanedSkill `json:"orphaned_skills"`
	PartialTaggingFailure bool            `json:"partial_tagging_failure"`
}

// CompileInput holds the parsed source data passed to ProfileCompiler.Compile.
// DataDir is intentionally absent — the caller reads files; the compiler
// operates on pure in-memory data.
type CompileInput struct {
	SkillsText   string           // raw content of skills.md
	Stories      []RawStory       // parsed from accomplishments-N.md files
	PriorProfile *CompiledProfile // nil on first run; used to carry deferred flags forward
}

// RawStory is an unparsed story read from an accomplishments file.
type RawStory struct {
	SourceFile string
	Text       string
}

// ExperienceRef is a lightweight career record used by the story creator to
// classify stories by job title without touching resume sections files.
type ExperienceRef struct {
	Role      string `json:"role"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

// StoryInput is the input to port.StoryCreatorService.Create.
type StoryInput struct {
	PrimarySkill string
	StoryType    StoryType
	JobTitle     string
	IsNewJob     bool
	StartDate    string // YYYY-MM or YYYY; required when IsNewJob=true
	EndDate      string // YYYY-MM, YYYY, or "present"; required when IsNewJob=true
	Situation    string
	Behavior     string
	Impact       string
}

// StoryOutput is returned by port.StoryCreatorService.Create.
type StoryOutput struct {
	SourceFile string // basename of the written accomplishments file, e.g. "accomplishments-2.md"
}
