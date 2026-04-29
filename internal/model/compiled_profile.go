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
// SourceFile is a source identifier: "onboard", a created_stories[].id string, or empty if
// unknown; informational only, not a join key.
type Story struct {
	ID         string    `json:"id"`
	SourceFile string    `json:"source_file"`
	Text       string    `json:"text"`
	Skills     []string  `json:"skills"`
	Format     string    `json:"format"`
	Type       StoryType `json:"type"`
	JobTitle   string    `json:"job_title"`
}

// OrphanedSkill is a skill label with no successfully-tagged supporting story.
type OrphanedSkill struct {
	Skill    string `json:"skill"`
	Deferred bool   `json:"deferred"`
}

// CompiledProfile is the derived artifact produced by compilation.
// Written to ~/.local/share/go-apply/profile-compiled.json. Never hand-edited.
type CompiledProfile struct {
	SchemaVersion  string          `json:"schema_version"`
	Skills         []string        `json:"skills"`
	CompiledAt     time.Time       `json:"compiled_at"`
	Stories        []Story         `json:"stories"`
	OrphanedSkills []OrphanedSkill `json:"orphaned_skills"`
}

// AssembleStory is one entry in AssembleInput.Stories.
// Exactly one of ID or Accomplishment must be set.
// ID references a story from the prior profile; Accomplishment is new story text.
// Source is supplied by the host: "onboard" for stories drawn from onboard_text,
// or the created_stories[].id string for created stories; empty means unknown.
type AssembleStory struct {
	ID             string   `json:"id,omitempty"`
	Accomplishment string   `json:"accomplishment,omitempty"`
	Tags           []string `json:"tags"`
	Source         string   `json:"source,omitempty"`
}

// AssembleInput is the host-provided input to ProfileCompiler.Compile.
// Skills are additive (union with prior); RemoveSkills explicitly removes.
type AssembleInput struct {
	Skills       []string         // additive: unioned with prior skills
	RemoveSkills []string         // explicit removals from prior skills
	Stories      []AssembleStory  // host-tagged stories
	PriorProfile *CompiledProfile // nil on first compile; required for ID resolution
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
	StoryID string // id of the entry written to created_stories in accomplishments.json
}
