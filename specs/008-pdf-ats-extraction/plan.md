# Implementation Plan: Honest Scoring Loop — PDF Renderer, ATS Extractor, Keyword-Survival Diff

**Branch**: `008-pdf-ats-extraction` | **Date**: 2026-04-25 | **Spec**: [spec.md](spec.md)

## Summary

Replace the stub `port.Renderer` (plain-text identity) and `port.Extractor` (identity) with real implementations: a pure-Go PDF renderer and a `pdftotext`-backed ATS extractor. Add a keyword-survival diff service that compares JD keywords against PDF-extracted text, surfacing dropped keywords. Wire into `preview_ats_extraction`. Add `go-apply doctor` preflight command. Remove two silent fallbacks in `preview_ats_extraction` that violate the constitution.

## Technical Context

**Language/Version**: Go 1.23+
**Primary Dependencies**:
- `github.com/go-pdf/fpdf` — pure-Go PDF generation, zero system deps, ATS-safe single-column achievable
- `pdftotext` (Poppler, system binary) — ATS-faithful text extraction; hard runtime dependency

**Storage**: No new storage. PDF bytes held in memory only — never written to disk.
**Testing**: `go test ./...`, extracted-text golden files, `go-apply doctor` integration test
**Target Platform**: macOS + Linux (wherever `pdftotext` is installable via package manager)
**Project Type**: CLI + MCP Server (hexagonal architecture)
**Performance Goals**: Render + extract pipeline latency is secondary to correctness; no specific target. Doctor check must complete in <2s.
**Constraints**: PDF bytes must not be written to disk during the pipeline (PII constraint). `pdftotext` must never be silently skipped — hard-fail only.
**Scale/Scope**: Single-user tool; no concurrency concerns for this feature.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Vertical Slicing | ✅ PASS | 3 independently testable user stories (US1 PDF render, US2 survival diff, US3 doctor) |
| II. Test-First Development | ✅ PASS | TDD required; extracted-text goldens for render+extract; doctor integration test |
| III. Hexagonal Architecture | ✅ PASS | New `port.PDFRenderer` interface; adapters in `internal/service/pdfrender/` and updated `internal/service/extract/`; no cross-layer imports |
| IV. No Silent Failures | ⚠️ PRE-EXISTING ISSUE | `preview_ats_extraction` has two silent fallbacks (session_tools.go:714, 719) that must be removed when real implementations land. Plan addresses this explicitly. |
| V. Observability | ✅ PASS | FR-009 mandates logging at extraction, pdftotext invocation, and survival diff computation |

**Constitution Issue IV note**: The silent fallbacks are pre-existing code (not introduced by this feature). This plan **removes** them as part of wiring in the real implementations. The `sections_used=false` path (no SectionMap sidecar) becomes an explicit `sections_required` error once the honest loop is the default.

## Project Structure

### Documentation (this feature)

```text
specs/008-pdf-ats-extraction/
├── plan.md              ← this file
├── research.md          ← Phase 0 output
├── data-model.md        ← Phase 1 output
├── quickstart.md        ← Phase 1 output
├── contracts/           ← Phase 1 output
└── tasks.md             ← Phase 2 output (/speckit.tasks)
```

### Source Code Changes

```text
internal/port/
├── render.go            ← unchanged (string renderer, tailor pipeline)
├── extract.go           ← unchanged ([]byte → string, fixed in Spec 007)
└── pdfrender.go         ← NEW: PDFRenderer interface

internal/service/
├── render/              ← unchanged (plain-text renderer stays)
├── pdfrender/           ← NEW: PDF renderer using go-pdf/fpdf
│   ├── pdfrender.go
│   └── pdfrender_test.go
├── extract/             ← MODIFIED: replace identity stub with pdftotext adapter
│   ├── extract.go       ← real pdftotext implementation
│   └── extract_test.go  ← updated + golden files
└── survival/            ← NEW: keyword-survival diff service
    ├── survival.go
    └── survival_test.go

internal/cli/
└── doctor.go            ← NEW: go-apply doctor subcommand

internal/mcpserver/
└── session_tools.go     ← MODIFIED: wire PDFRenderer, remove silent fallbacks,
                            add keyword_survival to previewData

internal/model/
└── survival.go          ← NEW: KeywordSurvival struct

go.mod                   ← MODIFIED: add github.com/go-pdf/fpdf dependency
go.sum                   ← MODIFIED: updated checksum
```
