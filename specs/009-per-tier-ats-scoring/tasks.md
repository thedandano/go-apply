# Tasks: Per-Tier Real-World ATS Scoring

**Input**: `specs/009-per-tier-ats-scoring/`
**Spec**: [spec.md](spec.md) · **Plan**: [plan.md](plan.md) · **Data Model**: [data-model.md](data-model.md)
**Branch**: `009-per-tier-ats-scoring`

## Format: `[ID] [P?] [Story] Description · subagent: <type>`

- **[P]**: Can run in parallel with sibling P-tasks (different files, no shared dependencies)
- **[Story]**: User story label (US1 = Latin-1 fix; US2 = PDF scoring)
- **subagent**: `sonnet` for logic-heavy work; `haiku` for mechanical wiring

---

## Closed Before Implementation

**User Story 3 — Threshold Calibration (P2)**: FR-010/FR-011 completed pre-implementation.
Mean delta +0.87 pts (PDF > plain-text) across 12 real JDs; threshold 70.0 unchanged.
SC-004 (routing decisions preserved) is validated by T015 regression test in Phase 4.

---

## Phase 1: Setup

**Purpose**: Promote `golang.org/x/sync` to direct dependency for `errgroup.WithContext` (needed by US2 T0 fan-out only). This can run in parallel with Phase 3 (US1 has no concurrency dependency).

- [ ] T001 Promote `golang.org/x/sync` to direct dependency via `go get golang.org/x/sync` and commit updated `go.mod` / `go.sum` · subagent: haiku

**Checkpoint**: `go.mod` lists `golang.org/x/sync` as a direct `require` entry.

---

## Phase 2: Foundational

No additional foundational work. Port interfaces (`port.PDFRenderer`, `port.Extractor`, `port.Scorer`) are unchanged.

---

## Phase 3: User Story 1 — Latin-1 Safe Resume Content (Priority: P1) 🎯 MVP

**Goal**: Replace the `validateLatin1Fields` rejection gate in `pdfrender.go` with a
`transliterateLatin1` pass AND replace the 4 hardcoded `" — "` em-dash literals in the
renderer with `" - "` (ASCII), so resumes render successfully regardless of typographic
characters in either user input or renderer format strings.

**Independent Test**: Render a PDF from a `model.SectionMap` that contains em-dashes,
smart quotes, bullets, non-breaking spaces, accented letters (é, ü), and at least one
unrepresentable character (U+2603). Assert: PDF file produced, no error, `?` in extracted
text for U+2603, slog.Warn emitted with codepoint and field name (NOT surrounding text).

**Constitution**: US1 ships as a standalone PR — do not include US2 changes.
**Parallel with T001**: Phase 3 has no dependency on `golang.org/x/sync`; start immediately.

> **TDD**: Write tests first (T002, T003). Confirm they FAIL before proceeding to T004.

### Tests for User Story 1

- [ ] T002 [P] [US1] Write table-driven unit tests for `transliterateLatin1` covering all FR-002 mappings (em-dash, en-dash, bullet U+2022, smart quotes ×4, ellipsis, non-breaking space U+00A0, hyphen variants U+2010/U+2011, accented Latin U+00C0–U+00FF, unrepresentable U+2603/U+1F600) in `internal/service/pdfrender/latin1_test.go` · subagent: sonnet
- [ ] T003 [P] [US1] Write integration tests for `RenderPDF` asserting: (a) successful render when Contact.Name, Skills flat text, and Experience bullets each contain non-ASCII characters (US1 §scenarios 1–5); (b) Certifications/Publications/Speaking/OpenSource sections render without error before and after the renderer literal fix in T006 — in `internal/service/pdfrender/pdfrender_test.go` · subagent: sonnet

### Implementation for User Story 1

- [ ] T004 [US1] Implement `transliterateLatin1(sections *model.SectionMap) model.SectionMap` with the FR-002 mapping table; function must return a value-type deep copy (never mutate the input pointer); for any character > U+00FF with no ASCII equivalent, log `slog.Warn` with codepoint and field name ONLY — do NOT include surrounding resume text in the log message (north-star: avoids leaking resume content to log sinks); apply to all string fields in `SectionMap` in `internal/service/pdfrender/latin1.go` · subagent: sonnet
- [ ] T005 [US1] Replace the `validateLatin1Fields()` call in `RenderPDF` with `sections = &transliterated` where `transliterated := transliterateLatin1(sections)` in `internal/service/pdfrender/pdfrender.go` · subagent: haiku
- [ ] T006 [US1] Replace the 4 hardcoded `" — "` em-dash literals in `pdfrender.go` with `" - "` (ASCII hyphen-space): lines ~754 (Certifications issuer), ~817 (Publications venue), ~852 (Speaking event), ~872 (Open Source role) — these renderer-generated strings bypass `transliterateLatin1` and will silently corrupt or fail without this fix in `internal/service/pdfrender/pdfrender.go` · subagent: haiku

**Checkpoint**: `go test ./internal/service/pdfrender/...` passes. All US1 acceptance scenarios green.
`grep -n '—' internal/service/pdfrender/pdfrender.go` returns only comment lines, no string literals.

---

## Phase 4: User Story 2 — Consistent ATS Score at Every Tier (Priority: P1)

**Goal**: Replace the `pdftotext` subprocess extractor with `ledongthuc/pdf` (pure-Go,
already in `go.mod`), implement the `scoreSectionsPDF` shared helper, and wire T0/T1/T2
handlers to use it so all three tiers score from PDF-extracted text.

**Dependency**: T001 must be complete (errgroup). US1 PR must be merged before this PR
opens (both touch the pdfrender package; T0 caller requires `transliterateLatin1` for
renders to succeed).

**Independent Test**: Call `scoreSectionsPDF` with a real `model.SectionMap` and a stub
`model.JDData`. Assert: non-empty `model.ScoreResult` returned; structured log entries for
render/extract/score stages emitted with session_id; `scoring_method = "pdf_extracted"` in
T0/T1/T2 JSON responses.

**Constitution**: US2 ships as a separate PR from US1.

> **TDD**: Write T006 and T007 first (test new functions in new files; confirm they FAIL
> at compile or assertion before writing T008). T010 requires T009's signature to exist
> first — author T010 after T009 is committed but before T011/T012 wire it in. T010 must
> fail with a runtime/assertion error (not a compile error) to be a meaningful red test.

### Tests for User Story 2

- [ ] T007 [P] [US2] Write unit tests for the new `extract.Service` implementation covering: normal PDF bytes produce non-empty text; empty bytes produce `("", nil)`; corrupt PDF bytes produce an error — in `internal/service/extract/extract_test.go` · subagent: sonnet
- [ ] T008 [P] [US2] Write unit tests for `scoreSectionsPDF` covering: happy path emits render/extract/score log entries with session_id; empty extracted text returns a labelled error; extraction failure propagates error — in `internal/mcpserver/score_pdf_test.go` · subagent: sonnet

### Implementation for User Story 2

- [ ] T009 [P] [US2] Replace the `pdftotext` subprocess body in `extract.Service.Extract` with `ledongthuc/pdf`: `r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))`, then `tr, err := r.GetPlainText()` (returns `(io.Reader, error)`), then `b, err := io.ReadAll(tr)`, then `text := string(b)`. Remove LookPath check, 30s timeout, and all subprocess logic. Add `import "io"`. Signature and `port.Extractor` interface unchanged — in `internal/service/extract/extract.go` · subagent: sonnet
- [ ] T010 [P] [US2] Implement `scoreSectionsPDF(ctx, sections, label, sessionID, jd, cfg, deps) (model.ScoreResult, error)` with `slog.InfoContext`/`slog.ErrorContext` entries at render/extract/score/failed stages each including `"session_id", sessionID`; check `len(text)==0` and return labelled error; define `ScoringMethodPDFExtracted = "pdf_extracted"` as package-level constant — in `internal/mcpserver/score_pdf.go` · subagent: sonnet
- [ ] T011 [US2] Write failing tests asserting T0 concurrent failure (one resume extraction fails → whole T0 call errors), T1 `scoring_method == "pdf_extracted"` in JSON response, T2 `scoring_method == "pdf_extracted"` in JSON response — in `internal/mcpserver/session_tools_test.go` (author AFTER T010 is committed, BEFORE T012/T013 wire it in) · subagent: sonnet
- [ ] T012 [US2] Modify T0 handler (`HandleSubmitKeywords`) to fan out `scoreSectionsPDF` calls via `errgroup.WithContext` with one goroutine per resume; pass `sessionID` to each call; collect results into pre-allocated slice; propagate first error — in `internal/mcpserver/session_tools.go` · subagent: sonnet
- [ ] T013 [US2] Modify T1 handler (`HandleSubmitTailorT1`) and T2 handler (`HandleSubmitTailorT2`) to call `scoreSectionsPDF` (passing `sessionID`) instead of plain-text `ScoreResume`; set `ScoringMethod = ScoringMethodPDFExtracted` on each response struct — in `internal/mcpserver/session_tools.go` · subagent: haiku

**Checkpoint**: `go test ./...` and `go test -race ./...` pass. All US2 acceptance scenarios green.
`grep -r "pdftotext\|exec.Command\|exec.LookPath" internal/` returns no matches.

---

## Final Phase: Polish & Cross-Cutting Concerns

- [ ] T014 Run `grep -r 'port\.Extractor\|Extractor\.Extract' internal/` and confirm no unintended consumers outside `mcpserver` and `extract` packages before US2 PR opens · subagent: haiku
- [ ] T015 [US3] Write routing regression test: for a fixed (resume sections, JD, threshold=70.0) triple, assert `NextActionAfterT1` returns the same routing decision via the PDF-extracted scorer as it did via the plain-text scorer (SC-004 coverage) — in `internal/mcpserver/session_tools_test.go` · subagent: sonnet
- [ ] T016 Run full suite `go test ./... && go test -race ./...` and confirm 80% coverage gate passes at pre-commit · subagent: haiku

---

## Dependency Graph

```
T001 (go.mod/x-sync) ──────────────────┐
                                        │
Phase 3 (US1 — no sync needed)          │
  T002 [P] ── T004 ──┐                  │
  T003 [P] ──────────┤                  │
                     T005 ── T006       │
                                 └── [US1 PR merged]
                                              └── [unblocks Phase 4] ←── T001 ──┘
                                                  T007 [P] ── T009 [P] ──┐
                                                  T008 [P] ── T010 [P] ──┤
                                                  T011 (after T010) ─────┤
                                                  T012 ── T013            │
                                                  T014 ── T015 ── T016 ──┘
```

**Phase 3 and T001 run in parallel.** Neither blocks the other.

## Parallel Execution Map

| Parallelizable group | Tasks | Why safe |
|---------------------|-------|----------|
| Setup ‖ US1 tests | T001 ‖ T002 ‖ T003 | Different files; US1 needs no `golang.org/x/sync` |
| US1 tests | T002 ‖ T003 | Different files (`latin1_test.go` vs `pdfrender_test.go`) |
| US2 tests | T007 ‖ T008 | Different files (`extract_test.go` vs `score_pdf_test.go`) |
| US2 impl core | T009 ‖ T010 | Different files (`extract.go` vs `score_pdf.go`) |

T011 must follow T010 (needs T010's interface). T012/T013 are both in `session_tools.go` and share state — run sequentially.

## Implementation Strategy

**MVP scope**: US1 (T002–T006) — unblocks the pipeline for every downstream spec.
**US2**: Begin only after US1 PR is merged (pdfrender changes; renders must succeed for `scoreSectionsPDF` to work).
