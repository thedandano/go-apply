# Research: Tier 4 Schema Sections + Section-Registry Foundation

**Phase 0** — all NEEDS CLARIFICATION items from Technical Context resolved.

---

## Decision 1: Entry struct pattern for Tier 4 sections

**Decision**: Adopt `PublicationEntry` as the canonical template for all 6 Tier 4 entry structs.

**Rationale**: `PublicationEntry` (`internal/model/resume.go:151–156`) is the simplest typed entry in the model — it has named fields with JSON tags, `omitempty`, no bullet IDs, and no computed methods. All 6 Tier 4 sections need the same treatment: a typed struct with a small set of string fields. Using a consistent pattern keeps model validation and JSON round-trips uniform.

**Alternatives considered**:
- `map[string]any` blob: rejected — `interface{}` is banned by constitution III; loses type-safety and JSON schema clarity.
- Reusing `PublicationEntry` for all Tier 4 types: rejected — each section has domain-specific fields (e.g., `Patents` needs a patent number; `Languages` needs proficiency; `References` needs contact info). A single generic type would require overloading field names with documentation conventions instead of type contracts.
- Embedded base struct: rejected — adds indirection without benefit; the sections are not polymorphic; Go embedding would require accessor boilerplate.

**Field designs per struct** (confirmed by domain analysis):

| Struct | Fields |
|---|---|
| `LanguageEntry` | `Name string`, `Proficiency string` |
| `SpeakingEntry` | `Title string`, `Event string`, `Date string`, `URL string` |
| `OpenSourceEntry` | `Project string`, `Role string`, `URL string`, `Description string` |
| `PatentEntry` | `Title string`, `Number string`, `Date string`, `URL string` |
| `InterestEntry` | `Name string` (simple — just a label) |
| `ReferenceEntry` | `Name string`, `Title string`, `Company string`, `Contact string` |

All fields: `json:",omitempty"` + `yaml:",omitempty"`. No pointers (consistent with existing entries).

---

## Decision 2: Section-registry shape in `render.Service.Render`

**Decision**: Ordered `[]sectionWriter` slice where `sectionWriter` is an unexported struct with a `key string` (for future use) and a `write func(*strings.Builder, *model.SectionMap)` closure.

```go
type sectionWriter struct {
    key   string
    write func(b *strings.Builder, s *model.SectionMap)
}
```

The `Render` method initialises the slice once (package-level `var` or inline) and iterates it in order, calling each writer.

**Rationale**: 
- Order is the primary contract. A slice guarantees FIFO deterministic iteration. Go maps do not.
- Closures over `*model.SectionMap` fields are direct field accesses — no reflection, no runtime string lookups.
- Adding a new section = appending one `sectionWriter` entry. No switch arm, no other file touched.
- The `key` field is retained for future use (e.g., template-specific include/exclude lists in Spec C) but is not used in this spec.

**Alternatives considered**:
- `map[string]func(...)`: rejected — non-deterministic iteration order; would require a separate ordered-keys slice anyway.
- Reflection over `SectionMap` fields: rejected — opaque; breaks with field renames; no benefit over explicit registry.
- `switch` with a `reflect.Value` lookup: rejected — more complex than the problem requires; constitution III bans `interface{}` indirection.

---

## Decision 3: `port.Extractor.Extract` signature change

**Decision**: `Extract(data []byte) (string, error)` — receives raw binary (future: PDF bytes); returns extracted plain text.

**Rationale**: The current `string` signature was fine for the identity stub but blocks a real PDF extractor (PDF is binary; converting to `string` before extraction would corrupt multi-byte sequences). The change is 4 files / ~6 lines and costs nothing now; deferring it to Spec C means Spec C must change a shared interface mid-implementation.

**Blast radius confirmed**:
1. `internal/port/extract.go` — 1 line (interface definition)
2. `internal/service/extract/extract.go` — 1 line (stub: `func (s *Service) Extract(data []byte) (string, error) { return string(data), nil }`)
3. `internal/service/extract/extract_test.go` — 2 call sites (wrap input with `[]byte(...)`)
4. `internal/mcpserver/session_tools.go` — 2 call sites (lines 717, 735: wrap with `[]byte(rendered)`, `[]byte(rawText)`)

**No other callers** — confirmed by grep. No loader packages implement `port.Extractor`; they implement `port.DocumentLoader`.

---

## Decision 4: Render order for Tier 4 sections

**Decision**: Append Tier 4 sections after `Publications` in this order: `Languages → Speaking → Open Source → Patents → Interests → References`.

**Rationale**: Preserves all existing render order (Contact → Summary → Experience → Education → Skills → Projects → Certifications → Awards → Volunteer → Publications) and adds Tier 4 at the end. The FR-011 order discrepancy (`Experience → Education → Skills` in code vs. `Experience → Skills → Education` in spec 004) is deliberately out of scope — fixing it here would change render output for existing resumes and violate SC-006.

---

## Decision 5: `parseSectionsArg` key names

**Decision**: Six new allowed keys added to the `knownSections` allowlist in `resume_validate.go`:

| Section key | Canonical string | Notes |
|---|---|---|
| Languages | `"languages"` | |
| Speaking | `"speaking"` | (not `"speaking_engagements"` — shorter, consistent with existing keys) |
| Open Source | `"open_source"` | underscore convention matches existing keys |
| Patents | `"patents"` | |
| Interests | `"interests"` | |
| References | `"references"` | |

These keys must match `SectionMap` JSON tags exactly to ensure `parseSectionsArg` → model round-trip is lossless.

---

## No NEEDS CLARIFICATION items remain

All five design decisions above are resolved. Constitution Check passes (see plan.md). Phase 1 may proceed.
