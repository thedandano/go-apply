package model

import "fmt"

const CurrentSchemaVersion = 1

type ResumeFile struct {
	Label    string
	Path     string
	FileType string
}

type TailorTier int

const (
	TierNone    TailorTier = 0
	TierKeyword TailorTier = 1
	TierBullet  TailorTier = 2
)

type BulletChange struct {
	Original  string `json:"original"`
	Rewritten string `json:"rewritten"`
}

type TailorResult struct {
	ResumeLabel      string         `json:"resume_label"`
	TierApplied      TailorTier     `json:"tier_applied"`
	AddedKeywords    []string       `json:"added_keywords,omitempty"`
	RewrittenBullets []BulletChange `json:"rewritten_bullets,omitempty"`
	// BulletsAttempted is the number of keyword-matching bullets sent to the LLM
	// during a tier-2 pass. When > 0 and RewrittenBullets is empty, every LLM call
	// failed (vs. simply no bullets matching keywords).
	BulletsAttempted int          `json:"bullets_attempted,omitempty"`
	OutputPath       string       `json:"output_path,omitempty"`
	NewScore         ScoreResult  `json:"new_score"`
	TailoredText     string       `json:"-"`                     // post-cascade text for accurate re-score delta; not serialized
	Tier1Text        string       `json:"tier1_text,omitempty"`  // output of tier-1 keyword injection, always set when T1 runs
	Tier1Score       *ScoreResult `json:"tier1_score,omitempty"` // score of tier-1 text; set by pipeline after TailorResume returns
}

// ResumeChanges describes the mutations the tailor service applied to a resume.
type ResumeChanges struct {
	AddedKeywords    []string
	RewrittenBullets []BulletChange
}

// TailorOptions carries behaviour-controlling limits for the tailor service.
// Values come from AppDefaults; extracted by the CLI/MCP layer before calling TailorResume.
type TailorOptions struct {
	MaxTier2BulletRewrites int
}

// TailorInput groups all inputs for a single tailor pass.
type TailorInput struct {
	Resume              ResumeFile
	ResumeText          string // pre-extracted by the pipeline before calling TailorResume
	JD                  JDData
	ScoreBefore         ScoreResult
	AccomplishmentsText string
	SkillsRefText       string
	Options             TailorOptions
}

// SkillsKind discriminator
type SkillsKind string

const (
	SkillsKindFlat        SkillsKind = "flat"
	SkillsKindCategorized SkillsKind = "categorized"
)

// SkillsSection discriminated union
type SkillsSection struct {
	Kind        SkillsKind          `json:"kind"`
	Flat        string              `json:"flat,omitempty"`
	Categorized map[string][]string `json:"categorized,omitempty"`
}

// ContactInfo
type ContactInfo struct {
	Name     string   `json:"name"`
	Email    string   `json:"email,omitempty"`
	Phone    string   `json:"phone,omitempty"`
	Location string   `json:"location,omitempty"`
	Links    []string `json:"links,omitempty"`
}

// ExperienceEntry
type ExperienceEntry struct {
	Company   string   `json:"company"`
	Role      string   `json:"role"`
	StartDate string   `json:"start_date"`
	EndDate   string   `json:"end_date,omitempty"`
	Location  string   `json:"location,omitempty"`
	Bullets   []string `json:"bullets"`
}

// ID returns the bullet ID prefix for this entry at index i: "exp-<i>"
func (e *ExperienceEntry) ID(i int) string {
	return fmt.Sprintf("exp-%d", i)
}

// BulletID returns "exp-<entryIndex>-b<bulletIndex>"
func (e *ExperienceEntry) BulletID(entryIndex, bulletIndex int) string {
	return fmt.Sprintf("exp-%d-b%d", entryIndex, bulletIndex)
}

// EducationEntry
type EducationEntry struct {
	School    string `json:"school"`
	Degree    string `json:"degree"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
	Location  string `json:"location,omitempty"`
	Details   string `json:"details,omitempty"`
}

// ProjectEntry
type ProjectEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Bullets     []string `json:"bullets,omitempty"`
	URL         string   `json:"url,omitempty"`
}

// CertificationEntry
type CertificationEntry struct {
	Name   string `json:"name"`
	Issuer string `json:"issuer,omitempty"`
	Date   string `json:"date,omitempty"`
}

// AwardEntry
type AwardEntry struct {
	Title   string `json:"title"`
	Date    string `json:"date,omitempty"`
	Details string `json:"details,omitempty"`
}

// VolunteerEntry
type VolunteerEntry struct {
	Org       string   `json:"org"`
	Role      string   `json:"role"`
	StartDate string   `json:"start_date,omitempty"`
	EndDate   string   `json:"end_date,omitempty"`
	Bullets   []string `json:"bullets,omitempty"`
}

// PublicationEntry
type PublicationEntry struct {
	Title string `json:"title"`
	Venue string `json:"venue,omitempty"`
	Date  string `json:"date,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Tier 4 entry structs — preserve-only sections with no edit primitives.

type LanguageEntry struct {
	Name        string `json:"name,omitempty"        yaml:"name,omitempty"`
	Proficiency string `json:"proficiency,omitempty" yaml:"proficiency,omitempty"`
}

type SpeakingEntry struct {
	Title string `json:"title,omitempty" yaml:"title,omitempty"`
	Event string `json:"event,omitempty" yaml:"event,omitempty"`
	Date  string `json:"date,omitempty"  yaml:"date,omitempty"`
	URL   string `json:"url,omitempty"   yaml:"url,omitempty"`
}

type OpenSourceEntry struct {
	Project     string `json:"project,omitempty"     yaml:"project,omitempty"`
	Role        string `json:"role,omitempty"        yaml:"role,omitempty"`
	URL         string `json:"url,omitempty"         yaml:"url,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type PatentEntry struct {
	Title  string `json:"title,omitempty"  yaml:"title,omitempty"`
	Number string `json:"number,omitempty" yaml:"number,omitempty"`
	Date   string `json:"date,omitempty"   yaml:"date,omitempty"`
	URL    string `json:"url,omitempty"    yaml:"url,omitempty"`
}

type InterestEntry struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
}

type ReferenceEntry struct {
	Name    string `json:"name,omitempty"    yaml:"name,omitempty"`
	Title   string `json:"title,omitempty"   yaml:"title,omitempty"`
	Company string `json:"company,omitempty" yaml:"company,omitempty"`
	Contact string `json:"contact,omitempty" yaml:"contact,omitempty"`
}

// SectionMap structured representation of a résumé
type SectionMap struct {
	SchemaVersion  int                  `json:"schema_version"`
	Contact        ContactInfo          `json:"contact"`
	Experience     []ExperienceEntry    `json:"experience"`
	Summary        string               `json:"summary,omitempty"`
	Skills         *SkillsSection       `json:"skills,omitempty"`
	Education      []EducationEntry     `json:"education,omitempty"`
	Projects       []ProjectEntry       `json:"projects,omitempty"`
	Certifications []CertificationEntry `json:"certifications,omitempty"`
	Awards         []AwardEntry         `json:"awards,omitempty"`
	Volunteer      []VolunteerEntry     `json:"volunteer,omitempty"`
	Publications   []PublicationEntry   `json:"publications,omitempty"`
	Languages      []LanguageEntry      `json:"languages,omitempty"   yaml:"languages,omitempty"`
	Speaking       []SpeakingEntry      `json:"speaking,omitempty"    yaml:"speaking,omitempty"`
	OpenSource     []OpenSourceEntry    `json:"open_source,omitempty" yaml:"open_source,omitempty"`
	Patents        []PatentEntry        `json:"patents,omitempty"     yaml:"patents,omitempty"`
	Interests      []InterestEntry      `json:"interests,omitempty"   yaml:"interests,omitempty"`
	References     []ReferenceEntry     `json:"references,omitempty"  yaml:"references,omitempty"`
	Order          []string             `json:"order,omitempty"`
}

// ResumeRecord full resume as stored
type ResumeRecord struct {
	Label    string      `json:"label"`
	RawText  string      `json:"raw_text"`
	Sections *SectionMap `json:"sections,omitempty"`
}
