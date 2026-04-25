# Implementation Plan: Tier 4 Schema Sections + Section-Registry Foundation

**Branch**: `007-tier4-schema-sections` | **Date**: 2026-04-25 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `specs/007-tier4-schema-sections/spec.md`

## Summary

`go-apply` currently silently drops resume sections for researchers, public speakers, OSS contributors, patent holders, polyglots, and anyone whose resume includes non-standard sections. This feature adds six new Tier 4 section types to the resume model (`Languages`, `Speaking`, `OpenSource`, `Patents`, `Interests`, `References`), refactors `render.Service.Render`'s 10-arm hardcoded dispatch into an ordered registry, and changes `port.Extractor.Extract` to accept `[]byte` so Spec C can plug in a real PDF extractor without touching the interface. `preview_ats_extraction` gets full Tier 4 support for free — the handler is already section-agnostic.

## Technical Context

**Language/Version**: Go 1.23+  
**Primary Dependencies**: `internal/model`, `internal/service/render`, `internal/port`, `internal/service/extract`, `internal/mcpserver` — all in-repo; no new external dependencies  
**Storage**: YAML resume files in `~/.local/share/go-apply/inputs/` (read path only; no schema migration required — new fields are `omitempty` and round-trip cleanly from existing YAML)  
**Testing**: `go test -race ./...`, `go vet ./...`, `golangci-lint run`  
**Target Platform**: darwin/linux CLI + MCP server  
**Project Type**: CLI / MCP server  
**Performance Goals**: N/A — pure model extension and dispatch refactor; no hot paths affected  
**Constraints**: No external dependencies added. `port.Extractor` signature change must be backward-safe for stub callers (achieved with `[]byte(s)` cast at call sites).  
**Scale/Scope**: Single-user CLI; ~6 new structs + 1 registry slice + 4-file signature update

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. Vertical Slicing | ✅ PASS | Three independent user stories; each can be merged separately without breaking the others. US1 (Tier 4 model) is the P1 vertical slice — delivers user value on its own. |
| II. Test-First Development | ✅ PASS | TDD required. Given/When/Then scenarios defined in spec. Tests written before each implementation increment. Minimum 80% coverage enforced by pre-commit hook. |
| III. Hexagonal Architecture | ✅ PASS | All changes touch only `model/`, `service/render/`, `port/`, `service/extract/`, `mcpserver/`. No service imports presenter. No `interface{}`. No provider names. `port.Extractor` signature fix is a legitimate interface evolution. |
| IV. No Silent Failures | ✅ PASS | Empty Tier 4 slice → writer is a no-op (no empty header, not a failure). Unknown section key in `parseSectionsArg` → explicit validation error (existing behaviour extended). `Extract([]byte)` callers wrap with explicit cast, no fallback. |
| V. Observability | ✅ PASS | No new observable operations introduced. Logging paths unchanged. Section rendering is not a logged operation; no new structured log requirements. |

**Post-design re-check**: ✅ PASS — design (registry slice, entry structs, interface signature) introduces no new violations.

**No Complexity Tracking violations** — no constitutionally problematic patterns introduced.

## Project Structure

### Documentation (this feature)

```text
specs/007-tier4-schema-sections/
├── plan.md          ← this file
├── spec.md          ← feature specification
├── research.md      ← Phase 0: pattern survey + design decisions
├── data-model.md    ← Phase 1: 6 entry struct definitions + SectionMap fields
├── contracts/
│   ├── section-registry.md   ← registry slice contract
│   └── extractor-interface.md ← port.Extractor []byte contract
├── quickstart.md    ← Phase 1: end-to-end verification steps
└── tasks.md         ← Phase 2 output (NOT created by /speckit.plan)
```

### Source Code (files touched)

```text
internal/model/
├── resume.go              # +6 entry structs, +6 SectionMap fields
└── resume_validate.go     # +6 keys in knownSections allowlist

internal/service/render/
└── render.go              # registry slice replaces 10 hardcoded calls; +6 Tier 4 writers

internal/port/
└── extract.go             # Extract(text string) → Extract(data []byte)

internal/service/extract/
├── extract.go             # stub: []byte → string identity
└── extract_test.go        # updated call sites

internal/mcpserver/
├── onboard.go             # parseSectionsArg learns 6 new keys
└── session_tools.go       # extractSvc.Extract([]byte(...)) at lines 717, 735
```

**Structure Decision**: Single-project Go CLI. All changes within `internal/`. No new packages created. No new top-level directories.
