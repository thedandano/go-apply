# Phase 1 Data Model: ATS-Aware Resume Tailoring

**Feature**: 004-ats-aware-tailoring
**Date**: 2026-04-24

All types are Go, in `internal/model/` unless noted. No `interface{}`; discriminated unions via typed structs with tagged variants (Constitution III).

---

## SectionMap

Structured representation of a résumé. One key per schema section; required keys are non-nullable, optional keys are pointer-or-slice types.

```go
// internal/model/resume.go (new additions)

const CurrentSchemaVersion = 1

type SectionMap struct {
    SchemaVersion  int                      `json:"schema_version"`
    Contact        ContactInfo              `json:"contact"`
    Experience     []ExperienceEntry        `json:"experience"`
    Summary        string                   `json:"summary,omitempty"`
    Skills         SkillsSection            `json:"skills,omitempty"`
    Education      []EducationEntry         `json:"education,omitempty"`
    Projects       []ProjectEntry           `json:"projects,omitempty"`
    Certifications []CertificationEntry     `json:"certifications,omitempty"`
    Awards         []AwardEntry             `json:"awards,omitempty"`
    Volunteer      []VolunteerEntry         `json:"volunteer,omitempty"`
    Publications   []PublicationEntry       `json:"publications,omitempty"`
    // Optional orchestrator-supplied override.
    Order          []string                 `json:"order,omitempty"`
}
```

### Required invariants
- `SchemaVersion == CurrentSchemaVersion` on load; mismatch returns `ErrSchemaVersionUnsupported`.
- `Contact.Name` is non-empty.
- `Experience` is non-nil (may be empty slice if candidate has none; validation distinguishes "required section missing" from "section empty").
- Section keys outside the declared set are rejected at `ValidateSectionMap` (unknown-key error).

---

## ContactInfo

```go
type ContactInfo struct {
    Name     string   `json:"name"`
    Email    string   `json:"email,omitempty"`
    Phone    string   `json:"phone,omitempty"`
    Location string   `json:"location,omitempty"`
    Links    []string `json:"links,omitempty"`
}
```

### Invariants
- `Name` is required and non-empty.
- `Email`, if present, must match `^[^@]+@[^@]+\.[^@]+$` (loose: MCP already validates at orchestrator side; this is a belt-and-suspenders check).

---

## ExperienceEntry

```go
type ExperienceEntry struct {
    // ID derived positionally; not persisted. Format: "exp-<index>".
    Company   string   `json:"company"`
    Role      string   `json:"role"`
    StartDate string   `json:"start_date"`           // ISO "YYYY-MM" or "YYYY"
    EndDate   string   `json:"end_date,omitempty"`   // ISO, "Present", or empty
    Location  string   `json:"location,omitempty"`
    Bullets   []string `json:"bullets"`
}
```

### Invariants
- `Company`, `Role`, `StartDate`, and non-nil `Bullets` are required. An entry with zero bullets is valid (empty slice).
- `StartDate` must parse as `YYYY` or `YYYY-MM`.
- `EndDate`, when present, must parse the same way OR equal literal `"Present"` (case-insensitive; normalized to `"Present"` on ingress).

### Derived properties
- `ID(i int) string` → `fmt.Sprintf("exp-%d", i)`.
- `BulletID(i, j int) string` → `fmt.Sprintf("exp-%d-b%d", i, j)`.
- YoE contribution: `duration(StartDate, EndDate) -> years.decimal`; merged across overlapping entries at the SectionMap level.

---

## SkillsSection (discriminated variant)

Skills may be submitted as either a flat string or a categorized map. Modeled as a tagged union:

```go
type SkillsSection struct {
    Kind SkillsKind `json:"kind"`
    // Populated when Kind == SkillsKindFlat.
    Flat string `json:"flat,omitempty"`
    // Populated when Kind == SkillsKindCategorized.
    Categorized map[string][]string `json:"categorized,omitempty"`
}

type SkillsKind string

const (
    SkillsKindFlat         SkillsKind = "flat"
    SkillsKindCategorized  SkillsKind = "categorized"
)
```

### Invariants
- Exactly one of `Flat` / `Categorized` is populated, consistent with `Kind`.
- Categorized values use category-name keys; category names are preserved verbatim for rendering under `Skills` (but the heading remains canonical `"Skills"`; categories become sub-labels).

### Edit-op targeting
- Flat: `target` is an existing skill token (substring, first match) or empty for `add`.
- Categorized: `target` is `"<category>/<token>"` for `replace`/`remove`; `"<category>"` for `add`.

---

## EducationEntry

```go
type EducationEntry struct {
    School    string `json:"school"`
    Degree    string `json:"degree"`
    StartDate string `json:"start_date,omitempty"`
    EndDate   string `json:"end_date,omitempty"`
    Location  string `json:"location,omitempty"`
    Details   string `json:"details,omitempty"`
}
```

---

## ProjectEntry

```go
type ProjectEntry struct {
    Name        string   `json:"name"`
    Description string   `json:"description,omitempty"`
    Bullets     []string `json:"bullets,omitempty"`
    URL         string   `json:"url,omitempty"`
}
```

---

## CertificationEntry

```go
type CertificationEntry struct {
    Name   string `json:"name"`
    Issuer string `json:"issuer,omitempty"`
    Date   string `json:"date,omitempty"`
}
```

---

## AwardEntry

```go
type AwardEntry struct {
    Title   string `json:"title"`
    Date    string `json:"date,omitempty"`
    Details string `json:"details,omitempty"`
}
```

---

## VolunteerEntry

```go
type VolunteerEntry struct {
    Org       string   `json:"org"`
    Role      string   `json:"role"`
    StartDate string   `json:"start_date,omitempty"`
    EndDate   string   `json:"end_date,omitempty"`
    Bullets   []string `json:"bullets,omitempty"`
}
```

---

## PublicationEntry

```go
type PublicationEntry struct {
    Title string `json:"title"`
    Venue string `json:"venue,omitempty"`
    Date  string `json:"date,omitempty"`
    URL   string `json:"url,omitempty"`
}
```

---

## Resume (repository-facing aggregate)

```go
// Existing model.ResumeFile stays (label + path + filetype).
// New: record combining the raw text with its parsed sections.
type ResumeRecord struct {
    Label    string
    RawText  string        // original submission
    Sections *SectionMap   // nil when not yet parsed — triggers ErrSectionsMissing
}
```

### Invariants
- `Label` is non-empty and contains no path separators (existing `validateLabel`).
- `Sections == nil` is a valid transient state (pre-feature records); every operation that requires sections MUST call `LoadSections` and propagate `ErrSectionsMissing` via the appropriate per-mode handler.

---

## EditEnvelope

```go
// internal/port/tailor.go (new types)

type EditOp string

const (
    EditOpAdd     EditOp = "add"
    EditOpRemove  EditOp = "remove"
    EditOpReplace EditOp = "replace"
)

type Edit struct {
    Section string `json:"section"`           // "skills", "experience", "summary", ...
    Op      EditOp `json:"op"`
    Target  string `json:"target,omitempty"`  // skill token or "exp-<i>-b<j>"
    Value   string `json:"value,omitempty"`   // add/replace content
}

type EditEnvelope struct {
    Edits []Edit `json:"edits"`
}

type EditResult struct {
    EditsApplied  []Edit          `json:"edits_applied"`
    EditsRejected []EditRejection `json:"edits_rejected"`
    NewSections   *model.SectionMap `json:"-"` // wire format emits `sections` at the handler layer
}

type EditRejection struct {
    Index  int    `json:"index"`
    Reason string `json:"reason"`
}
```

### Invariants
- `Edits` non-empty; handler enforces `len(Edits) >= 1`.
- `Section` must match a declared schema key; unknown keys → reject.
- `Op` must be one of the three constants; unknown → reject.
- Section/op combinations as documented in research §R6; mismatch → reject with `"invalid op for section X"`.

---

## AliasSet

```go
// internal/service/scorer/aliases.go

type AliasSet struct {
    // Canonical-form → list of accepted aliases.
    // Loaded once at scorer construction; immutable for lifetime of process.
    byCanonical map[string][]string
    // Reverse index built in constructor for O(1) bidirectional lookup.
    byAlias     map[string]string
}

func NewDefaultAliasSet() *AliasSet { ... }
func (a *AliasSet) Expand(keyword string) []string { ... } // returns [keyword, ...all siblings]
```

### Invariants
- Canonical forms and aliases are disjoint — no alias collides with a canonical of a different cluster.
- `Expand` always returns a slice whose first element is the input keyword (unchanged) so caller can distinguish the requested form.
- Case-insensitive lookup; stored keys are exact-case for user display.

---

## Renderer / Extractor

```go
// internal/port/render.go

type Renderer interface {
    // Render converts a SectionMap into the output text that downstream stages
    // (scoring, final output) consume. Today: markdown-ish plain text.
    // Future: PDF, DOCX — signature stays the same; output carries format tag.
    Render(sections *model.SectionMap) (string, error)
}

// internal/port/extract.go

type Extractor interface {
    // Extract converts a rendered artifact into the text an ATS would see.
    // Default impl is identity — input equals output.
    // Future: real PDF text extraction.
    Extract(rendered string) (string, error)
}
```

### Invariants
- `Renderer.Render` never returns empty string for a valid `SectionMap` (at minimum contact name must be present).
- `Extractor.Extract(s)` for the identity impl returns exactly `s`.
- Neither has side effects or I/O.

---

## Errors

```go
// internal/model/errors.go (or appended to resume.go)

var (
    ErrSectionsMissing           = errors.New("resume sections not available — re-onboard via add_resume with sections")
    ErrSchemaVersionUnsupported  = errors.New("resume sections schema version not supported")
    ErrNotSupportedInMCPMode     = errors.New("operation not available in MCP mode — orchestrator supplies sections directly")
)

type SchemaError struct {
    Field  string
    Reason string
}

func (e SchemaError) Error() string { ... }
```

### Propagation contract
- Headless mode: `ErrSectionsMissing` triggers automatic `Orchestrator.ParseSections`; transparent to user.
- MCP mode: `ErrSectionsMissing` surfaces as a typed error envelope `{"code": "sections_missing", "raw": "<raw text>", "hint": "call add_resume with sections"}` so the orchestrator can re-onboard without re-fetching.
- `SchemaError` surfaces at the handler boundary; callers include `edits_rejected`-style context where applicable.

---

## State transitions

```
 ResumeRecord lifecycle
 ┌─────────────┐   add_resume (sections)    ┌──────────────────┐
 │  (not       │───────────────────────────▶│ RawText +        │
 │   stored)   │                            │ Sections persist │
 └─────────────┘                            └───────┬──────────┘
                                                    │
                  pre-feature record (raw only)     │ load
                                                    ▼
                                            ┌──────────────────┐
                                            │ RawText only     │
                                            └───────┬──────────┘
                                                    │
              Headless: auto-parse via Orchestrator │  MCP: err envelope → orchestrator re-onboards
                                                    ▼
                                            ┌──────────────────┐
                                            │ RawText +        │
                                            │ Sections         │
                                            └──────────────────┘
```

```
 Session lifecycle (MCP)
 ┌────────┐   load_jd    ┌────────┐   submit_keywords   ┌────────┐
 │ none   │─────────────▶│ Loaded │────────────────────▶│ Scored │
 └────────┘              └────────┘                     └───┬────┘
                                                            │
                               submit_tailor (T1 or T2)     │
                            ┌───────────────────────────────┤
                            │                               │
                            ▼                               ▼
                      ┌──────────┐                    ┌──────────┐
                      │ T1       │─── submit_tailor ─▶│ T2       │
                      │ applied  │                    │ applied  │
                      └────┬─────┘                    └────┬─────┘
                           │                               │
                           │            finalize           │
                           └────────────┬──────────────────┘
                                        ▼
                                  ┌──────────┐
                                  │ Finalized│
                                  └──────────┘
```

(Existing state machine in `internal/mcpserver/session.go` already supports these transitions; the feature does not add new states.)

---

## Summary

| Entity | Location | New? |
|---|---|---|
| `SectionMap` + section entry types | `internal/model/resume.go` | new |
| `ResumeRecord` | `internal/model/resume.go` | new |
| `ContactInfo`, `SkillsSection`, `SkillsKind` | same | new |
| `EditOp`, `Edit`, `EditEnvelope`, `EditResult`, `EditRejection` | `internal/port/tailor.go` | new (extend existing port) |
| `AliasSet` | `internal/service/scorer/aliases.go` | new |
| `Renderer`, `Extractor` interfaces | `internal/port/{render,extract}.go` | new |
| `ErrSectionsMissing`, `ErrSchemaVersionUnsupported`, `ErrNotSupportedInMCPMode`, `SchemaError` | `internal/model/errors.go` | new |

Existing types reused unchanged: `ScoreResult`, `ScoreBreakdown`, `KeywordResult`, `JDData`, `ResumeFile`, `BulletChange`.
