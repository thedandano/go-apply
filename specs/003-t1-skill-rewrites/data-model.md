# Data Model: T1 Skill Section Rewrites

## Reused Types (no changes)

### `port.BulletRewrite`
```go
type BulletRewrite struct {
    Original    string
    Replacement string
}
```
Used as-is for `skill_rewrites` input. JSON tags already produce `original`/`replacement`.

## New Config Field

### `config.TailorDefaults.MaxTier1SkillRewrites`
```go
MaxTier1SkillRewrites int  // default: 5
```
Added to `TailorDefaults` struct in `internal/config/defaults.go`.
Default value `5` added to `internal/config/defaults.json`.

## New Function Signatures

### `tailor.ExtractSkillsSection`
```go
func ExtractSkillsSection(resumeText string) (section string, start, end int, found bool)
```
- `section`: verbatim body text of the Skills section (lines after the header line through
  the last content line; the header line itself is NOT included)
- `start`, `end`: line indices into the split-by-newline slice for splice-back
- `found`: false when no Skills header detected

### `tailor.ApplySkillsRewrites`
```go
func ApplySkillsRewrites(resumeText string, rewrites []port.BulletRewrite) (string, int, bool)
```
- Returns: modified resume text, substitution count, skills section found

## Response Shape Changes

### `submitKeywordsData` (submit_keywords response)
New field added:
```go
SkillsSection string `json:"skills_section,omitempty"`
```
Populated with the verbatim Skills section text of the best-scored resume.
Omitted when no Skills section exists.

### `t1Data` (submit_tailor_t1 response)
Field renamed:
```go
// Before
AddedKeywords      []string `json:"added_keywords"`

// After
SubstitutionsMade  int      `json:"substitutions_made"`
```
`SkillsSectionFound bool` retained unchanged.
