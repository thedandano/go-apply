# Implementation Plan: T1 Skill Section Rewrites

**Branch**: `003-t1-skill-rewrites` | **Date**: 2026-04-23 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/003-t1-skill-rewrites/spec.md`

## Summary

Replace the current T1 skill injection mechanism (append-only, unformatted line at bottom of
Skills section) with a string-replacement approach identical in shape to T2's `bullet_rewrites`.
The orchestrator receives the Skills section text from `submit_keywords`, writes
`{original, replacement}` pairs targeting existing category lines, and the server applies them
mechanically within the Skills section boundary. A configurable cap prevents section bloat.

## Technical Context

**Language/Version**: Go 1.26
**Primary Dependencies**: standard library only (`strings`, `encoding/json`); reuses `port.BulletRewrite`
**Storage**: N/A — stateless text transformation within existing session model
**Testing**: `go test ./... -race`, golangci-lint, 80% coverage gate
**Target Platform**: Linux/macOS — CLI binary and MCP server
**Project Type**: CLI tool / MCP server (hexagonal architecture)
**Performance Goals**: N/A — resume text is ~2 KB; transformation is O(n) string scan
**Constraints**: No new external dependencies; must pass `go vet` and `golangci-lint`; no
presenter package imports from service packages
**Scale/Scope**: Single resume text (~2 KB) per request; no concurrency concerns at this layer

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Vertical Slicing | ✅ PASS | 3 user stories, each independently testable and mergeable |
| II. Test-First | ✅ PASS | Unit tests for `ApplySkillsRewrites` and `ExtractSkillsSection` written before implementation; handler tests updated before handler |
| III. Hexagonal Architecture | ✅ PASS | Changes confined to `service/tailor`, `config`, `mcpserver`; reuses `port.BulletRewrite`; no presenter imports |
| IV. No Silent Failures | ✅ PASS | Empty array rejected; cap exceeded rejected with error code; section absent returns `skills_section_found: false`; unmatched `original` skipped but counted in `substitutions_made` |
| V. Observability | ✅ PASS | Existing handler structured logging covers new fields; `substitutions_made` and `skills_section_found` surfaced in response |

No violations. Complexity Tracking section not required.

## Project Structure

### Documentation (this feature)

```text
specs/003-t1-skill-rewrites/
├── plan.md              ← this file
├── research.md          ← Phase 0 (decisions log)
├── data-model.md        ← Phase 1 (type/config changes)
├── contracts/
│   └── submit_tailor_t1.md   ← Phase 1 (before/after tool schema)
└── tasks.md             ← Phase 2 (/speckit-tasks command)
```

### Source Code (affected files only)

```text
internal/
├── service/tailor/
│   ├── mechanical.go          add ApplySkillsRewrites (scoped to Skills section)
│   ├── mechanical_test.go     new tests for ApplySkillsRewrites
│   ├── tier1.go               add exported ExtractSkillsSection helper
│   └── tier1_test.go          new tests for ExtractSkillsSection
├── config/
│   ├── defaults.go            add MaxTier1SkillRewrites field to TailorDefaults
│   └── defaults.json          add "max_tier1_skill_rewrites": 5
└── mcpserver/
    ├── session_tools.go       update T1 handler (skill_rewrites input, cap check)
    │                          update submit_keywords handler (add skills_section)
    ├── session_tools_test.go  update T1 tests; add submit_keywords skills_section test
    ├── session_tools_retention_test.go  update skill_adds payloads → skill_rewrites
    ├── server.go              update submit_tailor_t1 tool description
    └── prompt.go              update skill_adds references → skill_rewrites (5 sites)
```

`port/` — no changes; `port.BulletRewrite` reused as-is for `skill_rewrites`.
`service/tailor/tailor.go` — no changes; CLI-mode `AddKeywordsToSkillsSection` retained.

---

## Phase 0: Research

*All unknowns resolved from prior codebase exploration. No external research required.*

See [research.md](research.md).

---

## Phase 1: Design & Contracts

### Data model changes

See [data-model.md](data-model.md).

### Interface contracts

See [contracts/submit_tailor_t1.md](contracts/submit_tailor_t1.md).

### Implementation approach (per user story)

**US1 — Inline replacement (core)**

1. Add `ExtractSkillsSection(text string) (section string, start, end int, found bool)` to
   `tier1.go` — reuses existing `isSkillsHeaderLine` + bounds scan, but returns indices so
   callers can splice the modified section back into the full text.
2. Add `ApplySkillsRewrites(resumeText string, rewrites []port.BulletRewrite) (string, int, bool)`
   to `mechanical.go`:
   - Call `ExtractSkillsSection` to isolate the Skills section
   - If not found: return original text, 0, false
   - Run `strings.ReplaceAll` for each rewrite within the section substring, in submission array order (FR-007)
   - Splice modified section back into full text
   - Return modified text, substitution count, true
3. Update `HandleSubmitTailorT1WithConfig` in `session_tools.go`:
   - Parse `skill_rewrites` as `[]port.BulletRewrite` (replaces `skill_adds []string`)
   - Validate non-empty
   - Validate `len(rewrites) <= cfg.Defaults.Tailor.MaxTier1SkillRewrites`
   - Call `tailor.ApplySkillsRewrites`
   - Update response struct (`SubstitutionsMade int`, keep `SkillsSectionFound bool`)

**US2 — skills_section in submit_keywords response**

1. In `HandleSubmitKeywordsWithConfig`, after scoring, load the best resume text via
   `loadBestResumeText` (same helper already used by T1/T2 handlers).
2. Call `tailor.ExtractSkillsSection` on the loaded text.
3. Add `SkillsSection string` to `submitKeywordsData` struct; populate if found, omit if not.

**US3 — length cap**

1. Add `MaxTier1SkillRewrites int` to `config.TailorDefaults` in `defaults.go`.
2. Add `"max_tier1_skill_rewrites": 5` to `defaults.json`.
3. Validation in handler (already described in US1 step 3).
