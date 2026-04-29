package model

// AccomplishmentsSchemaV1 is the only supported schema_version for accomplishments.json.
const AccomplishmentsSchemaV1 = "1"

// AccomplishmentsJSON is the on-disk representation of accomplishments.json.
// Written by onboard_user (sets OnboardText) and create_story (appends to CreatedStories).
type AccomplishmentsJSON struct {
	SchemaVersion  string         `json:"schema_version"`
	OnboardText    string         `json:"onboard_text"`
	CreatedStories []CreatedStory `json:"created_stories"`
}

// CreatedStory is a single entry in AccomplishmentsJSON.CreatedStories.
type CreatedStory struct {
	ID       string    `json:"id"`
	Skill    string    `json:"skill"`
	Type     StoryType `json:"type"`
	JobTitle string    `json:"job_title"`
	Text     string    `json:"text"`
}
