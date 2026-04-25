# Data Model: Tier 4 Schema Sections

---

## New entry structs (add to `internal/model/resume.go`)

Follow `PublicationEntry` pattern: flat struct, string fields, JSON + YAML tags with `omitempty`.

```go
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
```

---

## `SectionMap` additions (add to `internal/model/resume.go`)

Append after the existing `Publications []PublicationEntry` field:

```go
Languages   []LanguageEntry   `json:"languages,omitempty"   yaml:"languages,omitempty"`
Speaking    []SpeakingEntry   `json:"speaking,omitempty"    yaml:"speaking,omitempty"`
OpenSource  []OpenSourceEntry `json:"open_source,omitempty" yaml:"open_source,omitempty"`
Patents     []PatentEntry     `json:"patents,omitempty"     yaml:"patents,omitempty"`
Interests   []InterestEntry   `json:"interests,omitempty"   yaml:"interests,omitempty"`
References  []ReferenceEntry  `json:"references,omitempty"  yaml:"references,omitempty"`
```

Note: `open_source` uses underscore to match the canonical key name established in research.md Decision 5.

---

## `knownSections` additions (`internal/model/resume_validate.go`)

Add to the existing allowlist map (lines 9–20):

```go
"languages":   true,
"speaking":    true,
"open_source": true,
"patents":     true,
"interests":   true,
"references":  true,
```

---

## Validation rules

- **Empty slice**: valid. Writers are no-ops for empty slices — no empty section headers emitted.
- **Nil slice**: valid. `omitempty` handles nil in JSON/YAML round-trips.
- **Unknown keys**: `parseSectionsArg` already returns an error for keys not in `knownSections`. The Tier 4 keys are now in the allowlist, so no new validation logic needed.
- **No required fields within entries**: Tier 4 sections are fully optional and no per-field validation is applied within entries. Any entry struct is valid, even if all fields are empty strings. This is consistent with the "preserve" framing of US6 — the tool stores whatever the LLM provides.
- **No bullet IDs**: Tier 4 entries do not have bullet IDs (US6 framing is "preserve", not "edit"). `ExperienceEntry.BulletID` pattern is not adopted here.

---

## Section-registry type (add to `internal/service/render/render.go`)

```go
type sectionWriter struct {
    key   string
    write func(b *strings.Builder, s *model.SectionMap)
}
```

The `Render` method replaces its 10 hardcoded `writeX(...)` calls with iteration over an ordered `[]sectionWriter` slice. See `contracts/section-registry.md` for the full contract.
