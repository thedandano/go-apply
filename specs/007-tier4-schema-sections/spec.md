# Feature Specification: Tier 4 Schema Sections + Section-Registry Foundation

**Feature Branch**: `007-tier4-schema-sections`  
**Created**: 2026-04-25  
**Status**: Draft  
**Source**: Spec 004 deferred items US6 (FR-D06) + prep work for US5 (Spec C)

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Full-fidelity resume schema for unusual candidates (Priority: P1)

A job seeker with a research background lists publications, patents, and speaking engagements on their resume. The tool parses and preserves all of these sections through every stage of the pipeline, so the final output isn't missing content that was on the source resume.

**Why this priority**: Researchers, public speakers, OSS contributors, and authors currently have parts of their resume silently dropped. US6 from spec 004 was deferred until real user demand appeared — it is now explicitly requested.

**Independent Test**: Onboard a resume containing `Speaking Engagements`, `Patents`, `Languages`, and `Open Source` sections via `add_resume`. Run `preview_ats_extraction`. The extracted text must contain all four sections with their original content intact.

**Acceptance Scenarios**:

1. **Given** a resume with a `Languages: Go, Python, Rust` section, **when** onboarded and rendered, **then** the languages section appears in the rendered output and survives extraction unchanged.
2. **Given** a resume with a `Speaking Engagements` section containing two talks, **when** `preview_ats_extraction` is run, **then** both talk titles appear in the extracted text.
3. **Given** a resume with a `Patents` section, **when** rendered, **then** the patents section header and entries are included in the rendered text.
4. **Given** a resume with an `Interests` section and a `References` section, **when** rendered, **then** both sections appear after all other sections.
5. **Given** a resume with NO Tier 4 sections, **when** rendered, **then** output is identical to current behaviour (no regressions).

---

### User Story 2 — Section registry enables additive future templates (Priority: P2)

A future engineer adds a new resume section type or a new render template (multi-column, ATS-safe). They should be able to do so by adding a single entry to a registry, not by finding and updating multiple switch arms across the codebase.

**Why this priority**: Spec C (honest scoring loop) will add a real PDF renderer. Without a registry, every schema change requires a mirrored template update — guaranteeing Spec A ↔ Spec C collision. Fixing the dispatch now costs nothing; skipping it makes Spec C risky.

**Independent Test**: After the registry refactor, all existing tests pass unchanged. A code review confirms `render.Service.Render` contains no hardcoded section-name strings — sections are dispatched via the registry slice.

**Acceptance Scenarios**:

1. **Given** the registry-based `Render`, **when** a new `sectionWriter` entry is appended to the registry slice, **then** the new section is automatically rendered in the correct position without any other code changes.
2. **Given** a `SectionMap` with only Contact and Experience populated, **when** rendered, **then** all other sections are omitted and no empty headers appear.
3. **Given** the existing 10-section render order (Contact → Summary → Experience → Education → Skills → Projects → Certifications → Awards → Volunteer → Publications), **when** the registry refactor ships, **then** render output is byte-for-byte identical to pre-refactor output for all existing sections.

---

### User Story 3 — Binary-ready extractor interface (Priority: P3)

Spec C will plug in a real PDF extractor that receives raw PDF bytes — not a pre-decoded string. The `port.Extractor` interface must accept `[]byte` before Spec C starts, or Spec C will have to change a shared interface mid-implementation.

**Why this priority**: This is a one-line interface change with a 4-file blast radius. It is free to do now and painful to do mid-Spec-C.

**Independent Test**: After the change, `go build ./...` and `go vet ./...` pass. All callers compile with `[]byte(...)` wrapping. The stub extractor `service/extract/extract.go` converts `[]byte → string` and returns the identity.

**Acceptance Scenarios**:

1. **Given** the updated `port.Extractor` interface, **when** `extractSvc.Extract([]byte(rendered))` is called, **then** the stub returns the same string (identity behaviour preserved).
2. **Given** `service/extract/extract_test.go`, **when** updated to pass `[]byte` args, **then** all extract tests pass.

---

### Edge Cases

- What happens when a Tier 4 section entry has no items (empty slice)? The writer must be a no-op — no empty header appears in the output.
- What happens when `parseSectionsArg` receives a section key not in `knownSections`? It must return a validation error (existing behaviour; Tier 4 keys are now in the allowlist).
- What if a Tier 4 section contains a malformed entry (invalid JSON round-trip)? The model must fail validation, not silently corrupt.
- What if all Tier 4 entry fields are empty? Valid — Tier 4 sections are fully optional; no per-field validation is applied within entries. The section is omitted from render output when the slice is empty.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The `SectionMap` struct MUST include six new fields: `Languages`, `Speaking`, `OpenSource`, `Patents`, `Interests`, `References` (typed slices, `omitempty`).
- **FR-002**: `internal/model/resume_validate.go`'s `knownSections` allowlist MUST include the six new section keys.
- **FR-003**: `render.Service.Render` MUST dispatch sections via an ordered registry slice — no hardcoded section-name strings in the dispatch loop.
- **FR-004**: Each Tier 4 section MUST have a corresponding writer function; empty slices MUST produce no output. Rendered heading strings (all-caps, matching existing convention): `LANGUAGES` · `SPEAKING ENGAGEMENTS` · `OPEN SOURCE` · `PATENTS` · `INTERESTS` · `REFERENCES`.
- **FR-005**: `internal/mcpserver/onboard.go`'s `parseSectionsArg` MUST accept the six new section keys.
- **FR-006**: `port.Extractor.Extract` MUST have signature `Extract(data []byte) (string, error)`.
- **FR-007**: All callers of `extractSvc.Extract(...)` MUST pass `[]byte` arguments; no silent degradation.
- **FR-008**: `preview_ats_extraction` MUST return Tier 4 section content for resumes that include it (no handler change required — the handler is already section-agnostic; this is automatically satisfied by FR-001 + FR-003).

### Key Entities

- **`SectionMap`** (`internal/model/resume.go`): flat struct; gains 6 new typed slice fields.
- **`sectionWriter`** (`internal/service/render/render.go`): new named type `struct { key string; write func(*strings.Builder, *model.SectionMap) }` used in the ordered registry slice.
- **New entry structs** — all follow `PublicationEntry` pattern (title/detail/date/url fields, JSON tags, `omitempty`):
  - `LanguageEntry` — language name + proficiency
  - `SpeakingEntry` — talk title + event + date + url
  - `OpenSourceEntry` — project name + role + url + description
  - `PatentEntry` — title + number + date + url
  - `InterestEntry` — name (simple string entry)
  - `ReferenceEntry` — name + title + company + contact

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A resume with all six Tier 4 sections onboarded and rendered contains all six sections in output — zero sections silently dropped.
- **SC-002**: `preview_ats_extraction` on a Tier 4 resume returns all section content in extracted text.
- **SC-003**: `render.Service.Render` contains no hardcoded section-name strings in its dispatch loop (verified by code review).
- **SC-004**: `go test -race ./internal/model/... ./internal/service/render/... ./internal/service/extract/... ./internal/mcpserver/...` passes.
- **SC-005**: `go vet ./... && go build ./...` clean after `Extract([]byte)` signature change.
- **SC-006**: Render output for a resume with only existing sections is byte-for-byte identical before and after the registry refactor. **Test strategy**: write a dual-render test in `internal/service/render/render_test.go` — one call uses a test helper that invokes the pre-refactor hardcoded sequence (`writeContact`, `writeSection`/`writeExperience`, …, `writePublications` called directly), a second call uses the post-refactor `Render`; both receive the same fixture `SectionMap`. The test asserts `got == want` as a string comparison. The test helper is committed alongside the implementation and removed in a follow-up once the registry is the sole path.

## Assumptions

- In MCP mode, section content is supplied by the LLM via `parseSectionsArg`; there is no Go-side header-recognition parser to update. The onboarding package (`internal/service/onboarding/`) does not parse resume section headings.
- Tier 4 sections do NOT need edit primitives in this spec. T1 whitelist stays `"skills"`, T2 whitelist stays `"experience"`, `ApplyEdits` stays a 2-arm switch.
- Tier 4 sections append after `Publications` in the render order: Languages → Speaking → Open Source → Patents → Interests → References.
- The FR-011 order discrepancy (`Experience → Education → Skills` in code vs. `Experience → Skills → Education` in spec 004) is explicitly out of scope for this spec.
- `knownSectionKeywords` in `internal/service/tailor/tier1.go` is out of scope for this spec.
- **`SectionMap.Order` field** (`internal/model/resume.go:171`): This field is stored and validated (unknown keys in `Order` are rejected in `resume_validate.go:39–43`) but is **not consulted by `render.Service.Render`**. Render order is determined solely by the registry slice index. Adding the 6 Tier 4 keys to `knownSections` automatically makes them valid in `Order` with no further change required. The registry refactor does not change this behaviour.
- **`References: Available upon request`**: A resume with a plain "Available upon request" references section maps to `[]ReferenceEntry{{Name: "Available upon request"}}`. This is valid — no per-field validation is applied within Tier 4 entries (see Clarifications §Session 2026-04-25).

## Clarifications

### Session 2026-04-25

- Q: What heading strings do the 6 Tier 4 section writers emit? → A: `LANGUAGES` · `SPEAKING ENGAGEMENTS` · `OPEN SOURCE` · `PATENTS` · `INTERESTS` · `REFERENCES` (all-caps, matching existing convention)
- Q: Are Tier 4 entry fields required (e.g., must LanguageEntry.Name be non-empty)? → A: No — Tier 4 sections are fully optional; no per-field validation within entries. Any entry struct is valid.
