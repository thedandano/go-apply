# Tasks: Honest Scoring Loop — PDF Renderer, ATS Extractor, Keyword-Survival Diff

**Input**: Design documents from `specs/008-pdf-ats-extraction/`
**Branch**: `008-pdf-ats-extraction`
**Constitution**: TDD required (Red → Green → Refactor). Tests written before implementation.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 / US2 / US3
- All file paths are project-relative from repo root

---

## Phase 1: Setup

**Purpose**: Add the new Go module dependency and create golden file directory structure.

- [ ] T001 Run `go get github.com/go-pdf/fpdf` to add the dependency; commit updated `go.mod` and `go.sum`
- [ ] T002 [P] Create directory `internal/service/pdfrender/testdata/golden/` (add `.gitkeep` placeholder so the directory is tracked)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: New model type and port interface — required before any user story can compile.

**⚠️ CRITICAL**: US1, US2, and US3 all depend on this phase being complete.

- [ ] T003 [P] Create `internal/model/survival.go` — define `KeywordSurvival` struct: `Dropped []string json:"dropped"`, `Matched []string json:"matched"`, `TotalJDKeywords int json:"total_jd_keywords"`. Add `var _ = KeywordSurvival{}` compile check.
- [ ] T004 [P] Create `internal/port/pdfrender.go` — define `PDFRenderer` interface: `RenderPDF(sections *model.SectionMap) ([]byte, error)`. Add `// PDFRenderer converts a SectionMap into PDF bytes for ATS extraction.` doc comment.

**Checkpoint**: `go build ./...` must pass before proceeding to user stories.

---

## Phase 3: User Story 1 — Real PDF Generation (Priority: P1) 🎯 MVP

**Goal**: Replace stub renderer and identity extractor with real implementations. Remove all silent fallbacks from `preview_ats_extraction`.

**Independent Test**: `go test ./internal/service/pdfrender/... ./internal/service/extract/...` green; `TestHandlePreviewATSExtraction_NoSectionsSidecar_HardFails` passes.

### Tests (write first — must FAIL before T009/T010 are implemented)

- [ ] T005 [US1] Write failing tests in `internal/service/pdfrender/pdfrender_test.go`:
  - `var _ port.PDFRenderer = (*pdfrender.Service)(nil)` — compile-time assertion
  - `TestRenderPDF_NilInput_ReturnsError` — nil sections returns non-nil error
  - `TestRenderPDF_EmptyContact_ReturnsValidBytes` — minimal SectionMap (name only) returns non-empty bytes, no error
  - `TestRenderPDF_AllSections_GoldenExtractedText` — renders a fixed full SectionMap, calls a stub extractor to get text, diffs against `testdata/golden/full_resume.txt`; when `UPDATE_GOLDEN=1` is set, overwrites golden instead of asserting
  - `TestRenderPDF_InvalidUTF8_ReturnsError` — section field with `"\xff\xfe"` returns error
  - All tests must FAIL (package does not exist yet)

- [ ] T006 [P] [US1] Write failing tests in `internal/service/extract/extract_test.go` (update existing file):
  - `TestExtract_LookPathMissing_ReturnsDescriptiveError` — inject `lookPath` returning `exec.ErrNotFound`; assert error message contains `"go-apply doctor"`
  - `TestExtract_LookPathFound_SubprocessSuccess` — inject `lookPath` returning `"/usr/bin/pdftotext"` and `cmdFunc` that echoes stdin back to stdout; assert extracted text equals input bytes as string
  - `TestExtract_SubprocessFailure_StderrSanitized` — inject `cmdFunc` that exits non-zero with 500-byte stderr containing non-printable bytes; assert returned error contains at most 256 printable bytes of stderr, no raw non-printable chars
  - `TestExtract_ContextTimeout_KillsSubprocess` — inject short timeout; subprocess that sleeps; assert context deadline error
  - `TestExtract_LogsAttemptAndInvocation` — use a test `slog.Handler` (set via `slog.SetDefault`) to capture log output; assert `slog` entries with keys `"extract.attempt"` and `"extract.invoke"` are emitted during a successful extraction
  - All tests must FAIL (Service struct lacks new fields)

- [ ] T007 [P] [US1] Write failing test `TestHandlePreviewATSExtraction_NoSectionsSidecar_HardFails` in `internal/mcpserver/session_tools_test.go` — inject a resume repo stub where `LoadSections` returns an error; assert response has `error_code == "no_sections_data"` and no `constructed_text`. Must FAIL (current code falls back to raw text).
  - `TestHandlePreviewATSExtraction_RenderFails_ReturnsRenderFailedCode` — inject a stub `port.PDFRenderer` that returns an error; assert response has `error_code == "render_failed"`; must FAIL before T012 is implemented
  - `TestHandlePreviewATSExtraction_ExtractFails_ReturnsExtractFailedCode` — inject a stub `port.Extractor` that returns an error; assert response has `error_code == "extract_failed"`; must FAIL before T012 is implemented

- [ ] T008 [P] [US1] Write integration tests (file: `internal/mcpserver/session_tools_integration_test.go`, build tag `//go:build integration`) — `TestHandlePreviewATSExtraction_HonestLoop_RealPdftotext` (requires `pdftotext` on PATH) and `TestHandlePreviewATSExtraction_PdftotextMissing_ErrorReferencesDoctorCmd` (inject `pdftotext` absence via lookPath stub in extractSvc). Mark all tests in this file with `//go:build integration`.

### Implementation

- [ ] T009 [US1] Implement `internal/service/pdfrender/pdfrender.go`:
  - `Service` struct implementing `port.PDFRenderer`; add `var _ port.PDFRenderer = (*Service)(nil)`
  - `New() *Service` constructor
  - `RenderPDF(sections *model.SectionMap) ([]byte, error)` — nil check, UTF-8 validation of all string fields before embedding, use `github.com/go-pdf/fpdf` with `Fpdf.SetFont` + `Fpdf.MultiCell` for text, iterate sections in canonical order (Contact, Summary, Experience, Education, Skills, Projects, Certifications, Awards, Volunteer, Publications, Languages, Speaking, OpenSource, Patents, Interests, References) — order mirrors `sectionRegistry` in `internal/service/render/render.go` but defined locally (no import of render package)
  - Return `nil, error("pdfrender: zero-length output")` if PDF bytes are empty after generation
  - Emit `slog.Info("pdfrender.render", "sections", <count>)` at entry and `slog.Info("pdfrender.done", "bytes", <len>)` at successful return; no logging on error path (error return is sufficient)

- [ ] T010 [US1] Implement `internal/service/extract/extract.go` — replace identity stub:
  - `Service` struct with `lookPath func(string)(string,error)` (default `exec.LookPath`), `cmdFunc func(context.Context,string,...string)*exec.Cmd` (default `exec.CommandContext`), `timeout time.Duration` (default 10s)
  - `New() *Service` constructor sets defaults
  - `Extract(data []byte) (string,error)`: call `lookPath("pdftotext")` — return `fmt.Errorf("extractor: pdftotext not found — run go-apply doctor to diagnose: %w", err)` on failure; invoke via `cmdFunc` as `pdftotext - -` with stdin pipe for data, stdout pipe for result; on non-zero exit, capture stderr, cap to 256 bytes, strip bytes < 0x20 and > 0x7e, wrap in error; never use `sh -c`
  - Emit `slog.Info("extract.attempt", "binary", "pdftotext")` before `lookPath` call; emit `slog.Info("extract.invoke")` before subprocess start; both per FR-009

- [ ] T011 [US1] Run `UPDATE_GOLDEN=1 go test ./internal/service/pdfrender/... -run TestRenderPDF_AllSections_GoldenExtractedText` to generate `internal/service/pdfrender/testdata/golden/full_resume.txt`; commit the golden file

- [ ] T012 [US1] Update `internal/mcpserver/session_tools.go`:
  - Add `pdfRenderSvc port.PDFRenderer = pdfrender.New()` package-level var alongside `renderSvc` and `extractSvc`
  - In `HandlePreviewATSExtractionWithConfig`: replace the `renderSvc.Render` + `extractSvc.Extract` block (lines 711–725) with `pdfRenderSvc.RenderPDF` → `extractSvc.Extract` — on any error return `stageErrorEnvelope` with code `"render_failed"` or `"extract_failed"` respectively; remove the `slog.Warn` fallback
  - Replace the `LoadSections` fallback block (lines 728–741) with a hard-fail: if `LoadSections` returns error, return `stageErrorEnvelope(sessionID, "preview_ats_extraction", "no_sections_data", "no structured resume data — load a resume with sections to use honest scoring", false)`
  - Remove the unused `rawText` / `loadBestResumeText` path from this handler

---

## Phase 4: User Story 2 — Keyword-Survival Diff (Priority: P2)

**Goal**: Add `keyword_survival` field to `preview_ats_extraction` response showing which JD keywords were dropped by the PDF pipeline.

**Independent Test**: `go test ./internal/service/survival/...` green; `TestHandlePreviewATSExtraction_KeywordSurvivalPresent` passes.

### Tests (write first — must FAIL before T015/T016 are implemented)

- [ ] T013 [P] [US2] Write failing tests in `internal/service/survival/survival_test.go` (new file):
  - `var _ = survival.Service{}` — package exists check
  - `TestDiff_AllKeywordsSurvive` — all JD keywords present in extracted text; assert `Dropped` empty, `Matched` has all keywords, `Total == len(keywords)`
  - `TestDiff_SomeKeywordsDropped` — half missing from extracted text; assert split lands in correct slices; total invariant holds
  - `TestDiff_EmptyKeywords_ReturnsZeroStruct` — empty keyword list; assert `{[], [], 0}`, non-nil slices
  - `TestDiff_DeduplicatesSameKeywordAcrossReqAndPref` — keyword list with duplicate entries; assert counted once in Total, appears once in Matched or Dropped
  - `TestDiff_CaseInsensitiveMatching` — keyword "Kubernetes", extracted text has "kubernetes"; assert in Matched
  - `TestDiff_NonNilSlices` — empty result slices must be `[]string{}` not `nil`
  - `TestDiff_LogsKeywordCounts` — use a test `slog.Handler`; assert a log entry with key `"survival.diff"` is emitted containing fields `"total"`, `"dropped"`, `"matched"` with correct integer values
  - All tests must FAIL (package does not exist)

- [ ] T014 [P] [US2] Write failing test `TestHandlePreviewATSExtraction_KeywordSurvivalPresent` in `internal/mcpserver/session_tools_test.go`:
  - Use existing test infrastructure; inject a scored session with known `ScoreResult.Keywords`
  - Assert response JSON contains `keyword_survival` field with `dropped`, `matched`, `total_jd_keywords` keys
  - Assert `total_jd_keywords` equals count of unique JD keywords from the scored session
  - Must FAIL (field not yet in previewData)

### Implementation

- [ ] T015 [US2] Implement `internal/service/survival/survival.go`:
  - `Service` struct (no fields); `New() *Service` constructor
  - `Diff(keywords []string, extractedText string) model.KeywordSurvival` — assumes caller has deduplicated `keywords`; build case-insensitive whole-word regexp per keyword (same pattern as scorer's `compileKeywordPattern`); classify each into Matched or Dropped; return `KeywordSurvival{Dropped: dropped, Matched: matched, TotalJDKeywords: len(keywords)}` with non-nil slice guarantee (`if dropped == nil { dropped = []string{} }`)
  - Emit `slog.Info("survival.diff", "total", len(keywords), "dropped", len(dropped), "matched", len(matched))` before return; per FR-009

- [ ] T016 [US2] Update `internal/mcpserver/session_tools.go`:
  - Add `survivalSvc = survival.New()` package-level var
  - In `HandlePreviewATSExtractionWithConfig`, after extraction succeeds: derive deduplicated keyword list from `sess.ScoreResult.Keywords` (ReqMatched + ReqUnmatched + PrefMatched + PrefUnmatched, deduped via seen-map); call `survivalSvc.Diff(keywords, pd.ConstructedText)`; assign result to `pd.KeywordSurvival`
  - Update `previewData` struct: add `KeywordSurvival model.KeywordSurvival \`json:"keyword_survival"\`` field; update `SectionsUsed` comment: `// SectionsUsed is always true in a success response (no fallback path exists after FR-005b).`

---

## Phase 5: User Story 3 — go-apply doctor Preflight (Priority: P3)

**Goal**: Add `go-apply doctor` CLI command that checks `pdftotext` availability and emits clear pass/fail output.

**Independent Test**: `go test ./internal/cli/... -run TestDoctor` green.

### Tests (write first — must FAIL before T018 is implemented)

- [ ] T017 [P] [US3] Write failing tests in `internal/cli/doctor_test.go` (new file):
  - `TestDoctor_PdftotextPresent_PrintsOK` — inject `lookPath` returning `"/usr/bin/pdftotext", nil`; capture stdout; assert output contains `"[OK]"` and `"pdftotext"`
  - `TestDoctor_PdftotextMissing_PrintsMISSING` — inject `lookPath` returning `"", exec.ErrNotFound`; assert output contains `"[MISSING]"` and installation hint (`"poppler"`)
  - `TestDoctor_PdftotextMissing_ExitCodeOne` — same as above; assert command returns exit code 1
  - All tests must FAIL (package does not exist)

### Implementation

- [ ] T018 [US3] Implement `internal/cli/doctor.go`:
  - `DoctorCmd` struct with `lookPath func(string)(string,error)` field
  - `NewDoctorCommand() *cobra.Command` constructor — sets `lookPath` to `exec.LookPath`
  - `Run`: call `lookPath("pdftotext")`; on success print `"[OK] pdftotext — <path>"`; on failure print `"[MISSING] pdftotext — install poppler-utils (Linux) or: brew install poppler (macOS)"` and return with exit code 1 via `os.Exit(1)` or `cobra.CheckErr`
  - Output goes to `cmd.OutOrStdout()`

- [ ] T019 [US3] Register doctor subcommand: in `internal/cli/root.go`, add `cmd.AddCommand(NewDoctorCommand())` alongside the existing `AddCommand` calls (after `NewVersionCommand`)

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T020 [P] Run `go vet ./internal/service/pdfrender/... ./internal/service/extract/... ./internal/service/survival/... ./internal/cli/...` and `golangci-lint run` on the same packages — fix any warnings before opening PR
- [ ] T021 [P] Run `go test -race ./...` — verify no data races introduced by new package-level vars (`pdfRenderSvc`, `survivalSvc`)
- [ ] T022 Run `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out` — confirm all new packages exceed 80% line coverage; adjust tests if any fall short

---

## Dependencies

```
T001 → T009 (go-pdf/fpdf must be in go.mod before pdfrender implementation compiles)
T003 → T005, T013 (KeywordSurvival type must exist before tests reference it)
T004 → T005, T009 (PDFRenderer interface must exist before pdfrender tests and impl)
T005 → T009 (tests written before impl — TDD red state)
T006 → T010 (tests written before impl — TDD red state)
T007 → T012 (test must fail before fallback-removal impl)
T009 → T011 (impl must exist before golden file can be generated)
T009 → T012 (pdfRenderSvc uses new pdfrender.Service)
T010 → T012 (extractSvc uses new extract.Service)
T011 → T022 (golden file must be committed before CI runs)
T013 → T015 (tests before impl — TDD)
T014 → T016 (test must fail before KeywordSurvival wired in)
T015 → T016 (survival.Service must exist before session_tools wires it)
T016 → T014 (test passes after wiring)
T017 → T018 (tests before impl — TDD)
T018 → T019 (command must exist before it can be registered)
T012, T016, T019 → T020, T021, T022 (all impl done before polish)
```

## Parallel Execution Opportunities

**After T003+T004 complete (foundational):**
- T005, T006, T007, T008 can all be written in parallel (different files)

**After T009+T010 complete:**
- T011 (golden gen) runs immediately
- T012 (session_tools wiring) starts independently
- T013, T014 (US2 tests) can be written in parallel with T011/T012

**After T015+T016 complete:**
- T017 (US3 tests) can be written in parallel with T020/T021 (polish)

## Implementation Strategy

**MVP = Phase 3 (US1) only**: After T012, `preview_ats_extraction` uses the honest render→extract pipeline with hard errors. The survival diff (US2) and doctor command (US3) are independently shippable increments.

**Suggested delivery order**: US1 → US2 → US3 → Polish → PR

## Summary

| Phase | Tasks | Story | Parallel? |
|-------|-------|-------|-----------|
| Setup | T001–T002 | — | T002 [P] |
| Foundational | T003–T004 | — | Both [P] |
| US1 (PDF render + extract) | T005–T012 | US1 | T006–T008 [P] with T005 |
| US2 (Survival diff) | T013–T016 | US2 | T013–T014 [P] |
| US3 (Doctor command) | T017–T019 | US3 | T017 [P] |
| Polish | T020–T022 | — | T020–T021 [P] |

**Total: 22 tasks** | MVP: T001–T012 (12 tasks)
