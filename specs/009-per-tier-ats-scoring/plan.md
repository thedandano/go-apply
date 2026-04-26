# Implementation Plan: Per-Tier Real-World ATS Scoring

**Branch**: `009-per-tier-ats-scoring` | **Date**: 2026-04-26 | **Spec**: [spec.md](spec.md)
**Input**: `specs/009-per-tier-ats-scoring/spec.md`

## Summary

Replace the Latin-1 rejection gate in the PDF renderer with a transliteration pass, then
replace the `pdftotext` subprocess extractor with `ledongthuc/pdf` (pure-Go, already in
`go.mod`). Wire a shared `scoreSectionsPDF` helper into T0/T1/T2 handlers so all three
tiers score from PDF-extracted text, not plain-text renders.

## Technical Context

**Language/Version**: Go 1.23+
**Primary Dependencies**: `github.com/go-pdf/fpdf` (renderer), `github.com/ledongthuc/pdf` (extractor — already in `go.mod`)
**Storage**: N/A (no new storage; session state is in-memory)
**Testing**: `go test ./...`, `go test -race ./...`, 80% coverage gate at pre-commit
**Target Platform**: macOS/Linux (no OS binary required after extractor swap)
**Project Type**: MCP server / CLI
**Performance Goals**: T0 concurrent renders bounded by slowest resume, not sum
**Constraints**: No OS subprocess; extraction must work without `pdftotext` installed
**Scale/Scope**: Single-user, one active session at a time; up to ~5 resumes per T0 call

## Constitution Check

*GATE: Must pass before Phase 0. Re-checked after Phase 1.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Vertical Slicing | ✅ PASS | Story 1 (Latin-1 fix) and Story 2 (PDF scoring) ship as separate PRs; each is independently testable end-to-end |
| II. Test-First | ✅ REQUIRED | Tests written before implementation for every new function; Red→Green→Refactor |
| III. Hexagonal Architecture | ✅ PASS | `port.Extractor` interface unchanged; only the adapter (`extract.Service`) internals change; `scoreSectionsPDF` depends only on port interfaces |
| IV. No Silent Failures | ✅ PASS | Extraction failure → hard error; no plain-text fallback; FR-008 removed per spec |
| V. Observability | ✅ REQUIRED | `scoreSectionsPDF` must emit structured log entries: operation, session_id, resume_label, extracted_byte_count, score_total |

No violations. No Complexity Tracking table needed.

## Project Structure

### Documentation (this feature)

```text
specs/009-per-tier-ats-scoring/
├── plan.md          ← this file
├── research.md      ← Phase 0 output
├── data-model.md    ← Phase 1 output
└── tasks.md         ← Phase 2 output (/speckit.tasks — not created here)
```

### Source Code Changes

```text
internal/
├── service/
│   ├── pdfrender/
│   │   ├── pdfrender.go        MODIFY — replace validateLatin1Fields() rejection
│   │   │                                with transliterateLatin1(sections) call
│   │   └── latin1.go           NEW    — transliteration table + transliterateLatin1()
│   └── extract/
│       └── extract.go          MODIFY — replace pdftotext subprocess with ledongthuc/pdf
└── mcpserver/
    ├── score_pdf.go             NEW    — scoreSectionsPDF() shared helper
    └── session_tools.go         MODIFY — T0 concurrent scoreSectionsPDF; T1/T2 use
                                          scoreSectionsPDF; ScoringMethod in responses
```

No model changes. `ScoringMethodPDFExtracted` is a package-level constant set in
`score_pdf.go`; all three handlers reference it — no string literals in response structs.

**Architectural notes:**
- The `extract.Service` swap is **system-wide** — every `port.Extractor` consumer
  inherits the new implementation. Run `grep -r "port.Extractor\|Extractor.Extract"
  internal/` before the PR lands to confirm no unintended consumers exist.
- `scoreSectionsPDF` is placed in `mcpserver` for this spec. If spec 010's session cache
  requires calling it outside MCP transport, it MUST be moved to `internal/service/scoring/`
  at that point to avoid duplication. See research.md Decision 5.

**Structure Decision**: Single-project (existing layout). No new packages introduced.
All new code goes into the two service packages already responsible for these concerns.
