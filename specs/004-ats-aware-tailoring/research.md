# Phase 0 Research: ATS-Aware Resume Tailoring

**Feature**: 004-ats-aware-tailoring
**Date**: 2026-04-24
**Status**: Complete — all NEEDS CLARIFICATION resolved

## Purpose

Resolve technical unknowns before design. The spec answered *what* and *why*; this document locks down *how*: data structure shapes, port boundaries, persistence format, validation strategy, and reuse decisions. Each section ends with a **Decision / Rationale / Alternatives** block.

---

## R1. YoE computation for Renderer tier selection

**Unknown**: FR-011 requires the Renderer to pick experience-forward or education-forward ordering based on a years-of-experience signal derived from `sections.experience`. How is YoE computed? What happens with ambiguous dates (`"Present"`, missing end date, overlapping roles)?

**Decision**:
- Compute YoE as the **sum of `end_date − start_date` across all experience entries**, with overlaps counted once (merge overlapping intervals before summing).
- `"Present"`, empty string, or missing `end_date` → substitute the current date.
- Unparseable or missing `start_date` on an entry → skip that entry (do not fail rendering).
- If `sections.experience` is absent, empty, or produces 0 parseable entries → treat as `<3 YoE` (entry-level tier).
- Threshold: `total_years >= 3.0` → experienced tier; otherwise entry-level tier.

**Rationale**:
- Simple, deterministic, no LLM call.
- Matches how the council's recruiter framed the decision (aggregate experience, not most-recent role).
- Overlap merging prevents a candidate with two concurrent part-time roles from over-counting.
- The `<3` fallback on unparseable dates is a safe default: entry-level ordering still produces a valid resume; experienced-tier ordering on a 0-experience candidate does not.

**Alternatives considered**:
- Using the orchestrator-supplied `years_of_experience` profile field: rejected because that field is user-entered at onboarding and may not reflect the current resume's content.
- "Most recent role start_date to today" as YoE: rejected because it penalizes career gaps and overweights the latest role.
- LLM-computed YoE: rejected as unnecessary complexity; deterministic arithmetic on structured dates is sufficient.

---

## R2. Section parser in Headless (CLI) mode — new port vs extending `Orchestrator`

**Unknown**: FR-001 requires Headless to auto-parse raw text into sections. Where does the parser live?

**Decision**:
- Add a new method `ParseSections(ctx, raw) (model.SectionMap, error)` to the existing `port.Orchestrator` interface (`internal/port/orchestrator.go`).
- The Headless implementation (`internal/service/orchestrator/`) adds the corresponding method, calling `port.LLMClient.ChatComplete` with a JSON-schema-constrained prompt.
- The MCP implementation does not need to satisfy this method because in MCP mode Claude supplies sections directly at onboarding — but to keep `port.Orchestrator` coherent, the MCP-side orchestrator stub returns `ErrNotSupportedInMCPMode`.

**Rationale**:
- Keeps the existing port/adapter topology. `Orchestrator` already owns LLM-driven decision points (ExtractKeywords, PlanT1, PlanT2, GenerateCoverLetter); sections parsing is the same shape.
- Avoids a new top-level port for a single method.
- The MCP stub's explicit `ErrNotSupportedInMCPMode` keeps the failure mode loud rather than silent (Constitution IV: No Silent Failures).

**Alternatives considered**:
- New `port.SectionParser` interface: rejected — one method, same dependency (LLMClient), same call site (onboarding). Splitting is premature abstraction.
- Embed parser logic directly in onboarding service: rejected — couples MCP onboarding (which doesn't need LLM) to CLI onboarding (which does).

---

## R3. Persistence format for sections

**Unknown**: `internal/repository/fs/resume.go` today lists resume files from `dataDir/inputs/`, with the filename label as the key. Where do sections live on disk?

**Decision**:
- Store each resume as a pair: `<label>.txt` (raw) and `<label>.sections.json` (sections), both in `dataDir/inputs/`.
- Extend `port.ResumeRepository` with:
  - `LoadSections(label) (model.SectionMap, error)` — returns `ErrSectionsMissing` when the `.sections.json` sidecar is absent.
  - `SaveSections(label, sections) error`.
- Keep `ListResumes()` backed by the `.txt` scan; sections existence is checked lazily on read.
- Continue to support `.docx`, `.pdf`, `.md` extensions for backwards compatibility — the raw extraction path for those formats is unchanged.

**Rationale**:
- Sidecar JSON keeps raw text readable/editable by a human, satisfies the spec's "persist both raw and sections" requirement, and survives schema migrations without rewriting the raw file.
- No new top-level directory; no breakage for existing users.
- `ErrSectionsMissing` is the typed error that drives Headless auto-reparse (R2) and the MCP "sections missing" error envelope.

**Alternatives considered**:
- Single JSON file `<label>.json` containing both raw and sections: rejected because `.txt` is the existing format and a human-editable drop-in is easier for debugging.
- Separate `sections/` subdirectory: rejected — no benefit over sidecar and creates an extra directory to manage.
- SQLite or BoltDB: rejected — the project is deliberately filesystem-backed.

---

## R4. Schema validation location

**Unknown**: Where do we validate that a sections payload has required keys (`contact`, `experience`), correct types, and no unknown keys?

**Decision**:
- Add `ValidateSectionMap(sections) error` as a pure function on the `model` package (alongside the `SectionMap` type definition).
- Called at three sites:
  1. MCP `onboard_user` / `add_resume` handlers — before `Onboarder.Run`.
  2. Headless onboarding service — after `Orchestrator.ParseSections` returns, before `SaveSections`.
  3. `submit_tailor_t1` / `submit_tailor_t2` — against the resulting post-edit sections, before rescoring.
- Validation errors are typed: `SchemaError { Field string; Reason string }`; wrapped with `fmt.Errorf("validate sections: %w", err)` per constitution.

**Rationale**:
- Pure function in `model` — no dependencies, no I/O, trivially unit-testable.
- Validating at every boundary that accepts a `SectionMap` (write path) enforces the invariant one place even if a new caller is added later.
- Post-edit validation prevents a malformed edit envelope from corrupting persisted state.

**Alternatives considered**:
- JSON Schema library (e.g., `xeipuuv/gojsonschema`): rejected — adds a dependency for a small fixed schema. Hand-rolled validation is ~50 lines.
- Validation in the repository layer only: rejected — errors surface too late; want to fail at the MCP tool boundary before the orchestrator thinks it succeeded.

---

## R5. Alias matching — data structure, injection, bidirectionality

**Unknown**: FR-007 requires bidirectional alias matching without false positives. Where do aliases live, and how are they matched?

**Decision**:
- New file `internal/service/scorer/aliases.go` with:
  ```go
  var defaultAliases = map[string][]string{
      "Apache Spark": {"PySpark"},
      "PostgreSQL":   {"Postgres"},
      "Kubernetes":   {"K8s"},
      "JavaScript":   {"JS"},
      "TypeScript":   {"TS"},
  }
  ```
- Add `expandKeyword(kw string) []string` that returns `kw` plus any known aliases (bidirectional: if `kw == "PostgreSQL"` → `["PostgreSQL", "Postgres"]`; if `kw == "Postgres"` → `["Postgres", "PostgreSQL"]`).
- Modify the `classify` closure in `scorer.go` to match against each expanded form via OR, using the existing `compileKeywordPattern` for each (preserves word-boundary correctness for `C++`, `.NET`, `Go`).
- A match is credited once per JD keyword regardless of how many alias forms are present on the resume (dedup).

**Rationale**:
- Hardcoded map keeps the surface small (≤20 pairs per assumptions). Config-driven expansion is deferred (FR-D05).
- Bidirectional lookup via the same table inverted in-memory — no need to enumerate both directions in the source map.
- Reusing `compileKeywordPattern` keeps alias matching and regular matching identical — alias handling can't drift from the core word-boundary logic.

**Alternatives considered**:
- Symmetric pairs in the source (`("A", "B"), ("B", "A")`): rejected — duplication invites drift.
- Canonicalize the resume text first (replace `PySpark` → `Apache Spark` everywhere), then match: rejected — destructive and loses the original form, which the response surfaces to the orchestrator.
- Fuzzy/edit-distance matching: rejected — high false-positive risk (`Go` vs `Docker` via 1 transposition).

---

## R6. Unified edit envelope — wire format, validation, op semantics

**Unknown**: FR-004 defines `{ edits: [{ section, op, target?, value? }] }` with `op` ∈ `add`/`remove`/`replace`. What exactly does each op mean per section, and what errors are returned?

**Decision**:

Per-section op semantics:

| Section | `op` | `target` | `value` | Semantics |
|---|---|---|---|---|
| `skills` (string) | `replace` | existing skill token | new token | Substitute first match |
| `skills` (string) | `add` | — | token to append | Append to skills string |
| `skills` (string) | `remove` | existing skill token | — | Remove first match |
| `skills` (categorized map) | `replace` | `"<category>/<token>"` | new token | Replace in that category |
| `skills` (categorized map) | `add` | `"<category>"` | token | Append to category |
| `skills` (categorized map) | `remove` | `"<category>/<token>"` | — | Remove from category |
| `experience` | `replace` | `exp-<i>-b<j>` | new bullet text | Replace bullet at entry `i`, bullet index `j` |
| `experience` | `add` | `exp-<i>` | bullet text | Append bullet to entry `i`'s bullets |
| `experience` | `remove` | `exp-<i>-b<j>` | — | Remove bullet `j` from entry `i` |
| `summary` | `replace` | — | new summary | Overwrite |
| `summary` | `add` | — | text to append | Append with separator |

Unknown targets, out-of-range indices, invalid ops, and bad section keys populate `edits_rejected: [{ index, reason }]` in the response. Valid edits in the same envelope still apply; total success is `len(edits) == len(edits_applied) && len(edits_rejected) == 0`.

**Rationale**:
- Single wire format for T1 and T2 per Q2 decision. Polymorphic target resolution (string token vs bullet ID vs category path) is straightforward.
- Partial success + `edits_rejected` surfaces every rejection explicitly — no silent skips (Constitution IV).
- Bullet IDs are positional (`exp-0-b2`), generated on the fly from the sections, so no persistence churn. Collisions don't exist within a single resume; the "UUID later" path (Q2 answer) stays open.

**Alternatives considered**:
- One endpoint per section (`edit_skills`, `edit_experience`): rejected — duplicates validation plumbing and diverges from the unified design.
- All-or-nothing transactional semantics: rejected — one bad edit in a batch shouldn't block the rest when they are logically independent. Surfacing rejections is more useful to the orchestrator than rolling back.

---

## R7. Renderer canonical labels + Extractor identity

**Unknown**: FR-011b requires canonical labels. What exactly are they, and how does the Renderer emit them?

**Decision**:

Canonical label map (spec §Clarifications §Q4):

```
contact        → "Contact"
summary        → "Summary"
experience     → "Work Experience"
skills         → "Skills"
education      → "Education"
projects       → "Projects"
certifications → "Certifications"
awards         → "Awards"
volunteer      → "Volunteer Experience"
publications   → "Publications"
```

Output format: markdown-ish plain text with `## <Canonical Label>` headings, blank line, then section body. Bullets use `- ` markers. This is what the Extractor's identity implementation passes through to the scorer.

**Rationale**:
- `##` headings match the existing `atsSectionPatterns` regex in `scorer.go:54` so the `ATSFormat` dimension credits the right sections.
- Canonical labels are the ones the ATS expert flagged as parseable by Workday/Greenhouse/Taleo.
- Plain text is what today's `Extractor` receives and passes through unchanged; when real PDF extraction plugs in later (FR-D02), the format shifts upstream but the Extractor contract doesn't.

**Alternatives considered**:
- Preserve the orchestrator's original labels (`"Skills & Abilities"`): rejected — ATS expert specifically flagged that non-canonical labels confuse Workday and Taleo's section classifiers.
- JSON output from Renderer: rejected — scorer expects text. The Renderer's job is to produce what the Extractor consumes, which today is what the scorer scores.

---

## R8. Renderer + Extractor port placement

**Unknown**: Where in the port/adapter topology do the new interfaces live?

**Decision**:
- `internal/port/render.go` — `Renderer` interface.
- `internal/port/extract.go` — `Extractor` interface.
- `internal/service/render/` — default plain-text implementation.
- `internal/service/extract/` — default identity implementation.
- Pipeline uses `Renderer` to produce text for scoring and final output; uses `Extractor` immediately after render. Today: `scoreText = extract(render(sections))`, which is just `render(sections)`. The seam is where FR-D02's real extractor plugs in.

**Rationale**:
- Mirrors existing port/adapter conventions (`internal/port/scorer.go`, `internal/service/scorer/`).
- Keeping Renderer and Extractor as separate packages (not combined) maps to the two distinct future concerns: PDF rendering (FR-D01) and PDF extraction (FR-D02) are owned by different libraries.

**Alternatives considered**:
- Single `Formatter` interface doing both render and extract: rejected — the whole point of the seam is that real rendering and real extraction are independent.

---

## R9. MCP envelope additions

**Unknown**: Spec requires `submit_keywords` to return the full sections map and a new `preview_ats_extraction` MCP tool. What exactly lands on the wire?

**Decision**:

`submit_keywords` response gains:
```json
{
  ... existing fields ...,
  "sections": { /* full SectionMap of best_resume */ },
  "schema_version": 1
}
```
- `sections` is **not** `omitempty` — absence of sections should raise the structured "sections missing" error at the handler entry (per FR-014 MCP-mode behavior), not silently omit the field.
- The existing `skills_section` field is removed (no longer needed since `sections.skills` is authoritative). This is a breaking change to the MCP wire format for the best case; the tool is pre-1.0 and the only external consumer is Claude.

`submit_tailor_t1` and `submit_tailor_t2` responses gain:
```json
{
  "previous_score": ...,
  "new_score": ...,
  "edits_applied": [{ "section": ..., "op": ..., "target": ..., "value": ... }],
  "edits_rejected": [{ "index": N, "reason": "..." }],
  "sections": { /* post-edit SectionMap */ },
  "next_action": "..."
}
```

New tool `preview_ats_extraction`:
- Input: `session_id`.
- Output: `{ "extracted_text": string, "schema_version": 1 }` — the output of `extract(render(session.sections))`. Today identical to what the scorer saw.

**Rationale**:
- Replaces the broken `skills_section` path with the full `sections` view, satisfying FR-009 and giving the orchestrator the structure it needs for the next edit cycle.
- Explicit `schema_version` gives future clients a way to detect tier-4 expansion.
- `preview_ats_extraction` is low-risk identity today but the API surface stays stable for the real-extraction feature later.

**Alternatives considered**:
- Keep `skills_section` alongside `sections`: rejected — duplication invites drift and the `omitempty` footgun was the whole problem.
- Omit `schema_version`: rejected — cheap to add now, expensive to retrofit later.

---

## R10. Testing strategy (TDD)

**Unknown**: What tests gate each layer, and which are unit vs integration vs E2E?

**Decision**:

| Layer | Test file | Type |
|---|---|---|
| `model.SectionMap` + `ValidateSectionMap` | `internal/model/resume_test.go` | unit |
| YoE computation | `internal/model/resume_test.go` | unit |
| `Renderer` default | `internal/service/render/render_test.go` | unit |
| `Extractor` identity | `internal/service/extract/extract_test.go` | unit |
| Alias table + `expandKeyword` | `internal/service/scorer/aliases_test.go` | unit |
| `classify` with aliases (PySpark ↔ Apache Spark, Postgres ↔ PostgreSQL, negative cases) | `internal/service/scorer/scorer_test.go` | unit |
| Unified edit applier | `internal/service/tailor/edit_test.go` | unit |
| `ParseSections` (Headless) | `internal/service/orchestrator/sections_test.go` | unit w/ fake LLMClient |
| Resume repository sections I/O | `internal/repository/fs/fs_test.go` | unit |
| MCP envelope (sections field, preview tool, error envelope on missing sections) | `internal/mcpserver/session_tools_test.go`, `onboard_test.go` | unit |
| End-to-end: PlayStation replay scenario | `internal/mcpserver/e2e_test.go` | E2E |

Each test is written **before** its production code per Constitution II. Coverage gate is 80%.

**Rationale**:
- Matches existing test topology in the repo.
- The E2E replay is the concrete SC-006 gate.

**Alternatives considered**: None — the layer-per-test mapping is direct.

---

## R11. Reuse inventory

Existing utilities to reuse rather than duplicate:

| Utility | Location | Used for |
|---|---|---|
| `compileKeywordPattern` | `internal/service/scorer/scorer.go:157` | Regex compilation inside alias-aware classifier |
| `isWordChar` | `internal/service/scorer/scorer.go:149` | Same |
| `atsSectionPatterns` | `internal/service/scorer/scorer.go:53` | Canonical label validation — the Renderer's output must match these |
| `model.BulletChange` | `internal/model/resume.go:17` | Reported in `edits_applied` shape for T2 bullet replacements |
| `port.BulletRewrite` | `internal/port/orchestrator.go:34` | Internal adapter between edit envelope and existing CLI T1/T2 paths during migration (remove at end of feature) |
| `logger.PayloadAttr`, `logger.Verbose()` | `internal/logger/` | Debug-gated payload logging for new tool handlers |
| `debugdump.DiffSection`, `debugdump.DiffText` | `internal/debugdump/` | Verbose-mode diff emission for edit envelope application |
| `envelopeResult`, `okEnvelope`, `stageErrorEnvelope` | `internal/mcpserver/envelope.go` | MCP response construction — do not reinvent |

Delete / replace at feature completion:
- `skillsHeaderRe` (`internal/service/tailor/tier1.go:11`) — superseded by sections map lookup.
- `ExtractSkillsSection` (`internal/service/tailor/tier1.go:17`) — no callers after cutover.
- `AddKeywordsToSkillsSection` (`internal/service/tailor/tier1.go:44`) — superseded by unified edit applier.
- `ApplySkillsRewrites` (`internal/service/tailor/mechanical.go:13`) — same.
- `ApplyBulletRewrites` (`internal/service/tailor/mechanical.go:40`) — same.
- `extractExperienceBullets`, `isExperienceHeader` (`internal/service/tailor/tier2.go:30-75`) — superseded by structured `sections.experience`.
- `isHeaderLine`, `knownSectionKeywords`, `knownCompoundHeaders` (`internal/service/tailor/tier1.go:101-167`) — retained only if still needed by the Headless raw-resume loader path; otherwise deleted.

---

## Summary — NEEDS CLARIFICATION resolution

| Unknown | Resolved in | Decision |
|---|---|---|
| YoE computation semantics | R1 | Sum of merged intervals; `<3` threshold; default to entry-level on failure |
| Headless parser location | R2 | Extend `port.Orchestrator` with `ParseSections` |
| Sections persistence | R3 | `.sections.json` sidecar in `dataDir/inputs/` |
| Schema validation strategy | R4 | Pure function in `model`; called at every write boundary |
| Alias structure | R5 | Hardcoded map; `expandKeyword` bidirectional; OR-match via `compileKeywordPattern` |
| Edit envelope op semantics | R6 | Polymorphic targets; partial success with `edits_rejected` |
| Canonical labels | R7 | Fixed map; markdown `##` headings |
| Port placement | R8 | `internal/port/{render,extract}.go` + `internal/service/{render,extract}/` |
| MCP envelope diffs | R9 | Add `sections` + `schema_version`; remove `skills_section`; new `preview_ats_extraction` tool |
| Test layering | R10 | Unit per-package + single E2E replay |

All Phase 0 unknowns resolved. Proceed to Phase 1.
