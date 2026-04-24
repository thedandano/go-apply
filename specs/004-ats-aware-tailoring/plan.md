# Implementation Plan: ATS-Aware Resume Tailoring

**Branch**: `004-ats-aware-tailoring` | **Date**: 2026-04-24 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/004-ats-aware-tailoring/spec.md`

## Summary

Replace the regex-based resume heading detection with a structured `SectionMap` parsed upstream (MCP: by the orchestrating LLM; Headless: by the local LLM via a new `Orchestrator.ParseSections` port). Unify T1 and T2 onto a single `Edit` envelope (`{section, op, target?, value?}`) applied by a stateless `Tailor.ApplyEdits` implementation. Introduce `Renderer` and `Extractor` ports (identity today, PDF tomorrow) so the rendered→extracted text pipeline is fixed now without requiring any caller change when real extraction is swapped in. Add bidirectional alias-aware matching to the scorer so `Apache Spark`↔`PySpark`, `Postgres`↔`PostgreSQL`, etc. are treated as the same keyword. Render order is YoE-driven by default (≥3 years → experience-forward; <3 → education-forward) with a per-resume `order` override honored verbatim. Canonical heading labels are enforced on output regardless of input labels.

Technical approach resolved in [research.md](./research.md); types in [data-model.md](./data-model.md); wire formats in [contracts/mcp-tools.md](./contracts/mcp-tools.md); port signatures in [contracts/ports.md](./contracts/ports.md); end-to-end verification in [quickstart.md](./quickstart.md).

## Technical Context

**Language/Version**: Go 1.26
**Primary Dependencies**:
- `github.com/mark3labs/mcp-go` v0.47.1 (MCP transport)
- `github.com/charmbracelet/log` (structured logging via `slog` compatible bridge)
- `github.com/spf13/cobra` (CLI)
- `github.com/chromedp/chromedp` (headless Chrome — already present for JD fetching; unused by this feature)
- `github.com/ledongthuc/pdf` (resume ingest — present; future real extraction lives behind the new `port.Extractor`)

**Storage**: Filesystem only. Resumes at `dataDir/inputs/<label>.{txt,docx,pdf,md}`; new sidecar `dataDir/inputs/<label>.sections.json` persisted atomically (temp + rename) with `config.FilePerm`. No database.

**Testing**:
- Unit: `go test ./...` (table-driven; ports mocked via hand-rolled fakes — no `interface{}`).
- Integration: `go test -tags integration ./...` (MCP wire format, renderer/extractor round-trip, FS sidecar lifecycle).
- Race: `go test -race ./...` required for session-store–touching tests.
- Coverage: ≥ 80 % at pre-commit per Principle II.

**Target Platform**: macOS + Linux CLI/daemon. No browser, no server hosting outside the user's machine. The MCP server is an stdin/stdout transport consumed by the orchestrator (Claude Code).

**Project Type**: single Go module — three presenters (TUI, Headless, MCP) fan out from a shared `internal/service/*` + `internal/port/*` core.

**Performance Goals**: `ApplyEdits` is pure in-memory mutation of a small `SectionMap` → sub-millisecond. `Renderer.Render` and `Extractor.Extract` (identity) similarly sub-millisecond. `submit_tailor_t1/t2` MCP handler p95 ≤ 200 ms excluding scorer re-run. Alias expansion is O(1) per keyword (reverse index built once at construction).

**Constraints**:
- No silent failures — every error wrapped with `fmt.Errorf("context: %w", err)`; typed error codes on every MCP error envelope.
- No `interface{}` — `SkillsSection` uses a typed `Kind` discriminator; edit operations use a typed `EditOp` constant set.
- No hardcoded provider names — the parser calls `port.LLMClient.ChatComplete`, not any named SDK.
- Schema version embedded in every persisted sidecar; mismatched versions return `sections_unsupported_schema` at load time.
- **TUI carve-out**: TUI mode is on a deprecation path and is explicitly out of scope. TUI continues to use the existing regex-based tailoring path unchanged — no new service or port code touches TUI presenter files.

**Scale/Scope**: single-user local installation; typical resume ~30 KB raw, SectionMap ~15 KB JSON. Session store holds at most a dozen entries in memory. No concurrent-writer guarantees required beyond the existing FS atomic-rename pattern.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### I. Vertical Slicing (NON-NEGOTIABLE) — ✅ PASS

The feature is naturally sliced into independently testable behaviors: sections ingest (US1–US2), schema-targeted T1/T2 (US3), alias scoring (US4), renderer/extractor seam + preview (US5), and deferred D-series stories remain deferred. Each US maps to a functional requirement cluster and delivers observable end-to-end behavior through either MCP wire calls or Headless CLI invocation. No horizontal "just the data model" increment is proposed.

### II. Test-First Development (NON-NEGOTIABLE) — ✅ PASS

Tests precede implementation at every layer per research §R10:
- Unit: `model.ValidateSectionMap`, `AliasSet.Expand`, `Tailor.ApplyEdits` op table, `Renderer.Render` canonical-label enforcement.
- Contract: each MCP tool in contracts/mcp-tools.md has a wire-format test before handler code is written.
- Integration: PlayStation replay (quickstart §11) covered by a tagged integration test.
- Coverage: scope of touched packages — `internal/model`, `internal/service/{render,extract,scorer,tailor,orchestrator}`, `internal/repository/fs`, `internal/mcpserver` — all to be ≥ 80 %.

### III. Hexagonal Architecture (NON-NEGOTIABLE) — ✅ PASS

All new surfaces live in `internal/port/`:
- `port.Renderer` / `port.Extractor` — new interfaces, concrete impls in `internal/service/render/` and `internal/service/extract/`.
- `port.Tailor.ApplyEdits` — extends existing port; implementation stays in `internal/service/tailor/`.
- `port.Orchestrator.ParseSections` — extends existing port.
- `port.ResumeRepository.{Load,Save}Sections` — extends existing port; adapter in `internal/repository/fs/`.

No presenter → service imports introduced. No `interface{}`. Discriminated union (`SkillsSection.Kind`) replaces what would otherwise require a type switch on `any`. Compile-time guards (`var _ port.Renderer = (*Service)(nil)`) enforced in every new adapter.

### IV. No Silent Failures (NON-NEGOTIABLE) — ✅ PASS

- Schema validation errors surface as `SchemaError{Field, Reason}`, propagated verbatim.
- Missing sidecar returns typed `ErrSectionsMissing`, handled per mode (MCP: typed envelope with `raw` payload; Headless: auto-parse).
- Edit rejections surface as `EditRejection{Index, Reason}` in the response envelope — never silently dropped.
- MCP envelope gains six new typed error codes (`missing_sections`, `invalid_sections`, `sections_missing`, `too_many_edits`, `invalid_edits`, `sections_unsupported_schema`) — no generic "internal error" fallback.
- The old `skills_section` silent-`omitempty` field is removed; replaced by full `sections` payload with explicit errors.

### V. Observability (NON-NEGOTIABLE) — ✅ PASS

- Every new MCP handler emits a structured `slog` entry with operation name, session ID, input counts (edit count, section keys modified), outcome, and elapsed time.
- Session ID is the correlation identifier for T1 → T2 → finalize traceability.
- Verbose mode (`logger.Verbose()`) dumps: the parsed `SectionMap`, pre/post-edit diffs, alias expansions that contributed to a match, and the rendered→extracted text pair.
- Behavior is debuggable from logs alone — no need to attach a debugger.
- Pre-commit and pre-push gates (lint, format, 80 % coverage, unit tests) remain unchanged and enforced.

**All five principles pass. No violations to justify.**

## Project Structure

### Documentation (this feature)

```text
specs/004-ats-aware-tailoring/
├── plan.md                      # This file (/speckit.plan output)
├── research.md                  # Phase 0 (11 decisions, §R1–R11)
├── data-model.md                # Phase 1 (Go types + invariants)
├── quickstart.md                # Phase 1 (11 verification scenarios)
├── contracts/
│   ├── mcp-tools.md             # Phase 1 (MCP wire formats)
│   └── ports.md                 # Phase 1 (Go interface signatures)
└── tasks.md                     # Phase 2 — created by /speckit.tasks, NOT here
```

### Source Code (repository root)

```text
internal/
├── model/
│   ├── resume.go                # +SectionMap, +ContactInfo, +ExperienceEntry, +SkillsSection, …
│   ├── resume_validate.go       # (new) ValidateSectionMap pure function
│   └── errors.go                # (new) ErrSectionsMissing, ErrSchemaVersionUnsupported, SchemaError
├── port/
│   ├── render.go                # (new) Renderer interface
│   ├── extract.go               # (new) Extractor interface
│   ├── tailor.go                # +EditOp, +Edit, +EditResult, +EditRejection, +ApplyEdits
│   ├── orchestrator.go          # +ParseSections
│   └── resume.go                # +LoadSections, +SaveSections
├── service/
│   ├── render/
│   │   ├── render.go            # (new) default markdown renderer, YoE-aware order
│   │   └── render_test.go
│   ├── extract/
│   │   ├── extract.go           # (new) identity extractor
│   │   └── extract_test.go
│   ├── tailor/
│   │   ├── apply_edits.go       # (new) ApplyEdits implementation
│   │   ├── tier1.go             # skillsHeaderRe DELETED
│   │   ├── tier2.go             # bullet ops use exp-<i>-b<j> IDs
│   │   └── mechanical.go        # ApplySkillsRewrites / ApplyBulletRewrites DELETED
│   ├── orchestrator/
│   │   ├── orchestrator.go      # +ParseSections (Headless adapter)
│   │   └── parse_sections.go    # (new) JSON-schema-constrained prompt
│   └── scorer/
│       ├── aliases.go           # (new) AliasSet with bidirectional index
│       └── scorer.go            # classify() calls AliasSet.Expand
├── repository/
│   └── fs/
│       └── resume.go            # +LoadSections / +SaveSections (sidecar atomic rename)
└── mcpserver/
    ├── onboard.go               # sections required in MCP mode
    ├── session_tools.go         # submit_keywords returns sections; skills_section removed
    ├── tailor_tools.go          # unified edits[] envelope; edits_applied / edits_rejected
    └── preview.go               # (new) preview_ats_extraction handler
```

**Structure Decision**: single-module Go project. All new code lives under `internal/` respecting the existing hexagonal split: `model` (pure types + validation) → `port` (interfaces) → `service` (implementations) → `repository` / `mcpserver` (adapters). No new top-level package trees, no cross-layer imports introduced. Deletions (`skillsHeaderRe`, `ApplySkillsRewrites`, `ExtractSkillsSection`) happen at feature-end in the same PR sequence as the replacement — no flag-gated dead code left behind per Operating Mode.

## Complexity Tracking

> Constitution Check shows no violations; this table is empty by design.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| *(none)*  | —          | —                                    |
