# Tasks: Tier 4 Schema Sections + Section-Registry Foundation

**Input**: Design documents from `specs/007-tier4-schema-sections/`
**Prerequisites**: plan.md ✅ · spec.md ✅ · research.md ✅ · data-model.md ✅ · contracts/ ✅ · quickstart.md ✅

**TDD required** (constitution II): write failing tests before each implementation block.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no blocking dependencies)
- **[Story]**: Maps to user stories US1/US2/US3 from spec.md

---

## Phase 1: Setup

**Purpose**: Verify baseline passes before any changes.

- [ ] T001 Run `go test -race ./...` and confirm all tests green before modifying any files

**Checkpoint**: Baseline green — story phases can begin.

---

## Phase 3: User Story 1 — Tier 4 Model Additions (Priority: P1) 🎯 MVP

**Goal**: Add 6 new entry structs and `SectionMap` fields so the resume model can represent Languages, Speaking, OpenSource, Patents, Interests, and References. Update `knownSections` allowlist and `parseSectionsArg` so the MCP layer accepts the new keys.

**Independent Test**: `go test -race ./internal/model/... ./internal/mcpserver/...` passes; a JSON round-trip of a `SectionMap` containing all 6 new slice fields produces identical output.

### Tests for User Story 1 (write first — must FAIL before implementation) ⚠️

- [ ] T002 [P] [US1] Add JSON round-trip tests for all 6 new entry structs (`LanguageEntry`, `SpeakingEntry`, `OpenSourceEntry`, `PatentEntry`, `InterestEntry`, `ReferenceEntry`) in `internal/model/resume_test.go`
- [ ] T003 [P] [US1] Add `knownSections` allowlist test asserting `"languages"`, `"speaking"`, `"open_source"`, `"patents"`, `"interests"`, `"references"` are valid keys in `internal/model/resume_validate_test.go`
- [ ] T004 [P] [US1] Add `parseSectionsArg` acceptance test for all 6 new keys in `internal/mcpserver/onboard_sections_test.go`

### Implementation for User Story 1

- [ ] T005 [US1] Add 6 entry structs and 6 `SectionMap` fields (after `Publications`) to `internal/model/resume.go` — follow `PublicationEntry` pattern (flat struct, string fields, `json:"...,omitempty"` + `yaml:"...,omitempty"`, no pointers)
- [ ] T006 [US1] Add 6 keys to `knownSections` map in `internal/model/resume_validate.go` (`"languages"`, `"speaking"`, `"open_source"`, `"patents"`, `"interests"`, `"references"`)
- [ ] T007 [P] [US1] Add 6 new key cases to `parseSectionsArg` in `internal/mcpserver/onboard.go`

**Checkpoint**: US1 complete — `go test -race ./internal/model/... ./internal/mcpserver/...` fully green. US2 may now start (it needs the new `SectionMap` fields).

---

## Phase 4: User Story 2 — Section Registry Refactor (Priority: P2)

**Goal**: Replace `render.Service.Render`'s 10 hardcoded `writeX(...)` calls with an ordered `[]sectionWriter` slice. Add 6 Tier 4 writer functions. Render output for existing sections must be byte-for-byte identical (SC-006).

**Independent Test**: After refactor, all pre-existing render tests pass unchanged; dual-render test (hardcoded helper vs. registry) asserts identical output; each Tier 4 heading (`LANGUAGES`, `SPEAKING ENGAGEMENTS`, `OPEN SOURCE`, `PATENTS`, `INTERESTS`, `REFERENCES`) appears when the corresponding slice is non-empty.

### Tests for User Story 2 (write first — must FAIL before implementation) ⚠️

- [ ] T008 [US2] Write dual-render test in `internal/service/render/render_test.go`: build a pre-refactor helper that calls the 10 existing `writeX` functions directly and compare its output to `Render` on the same fixture — test must FAIL (no registry yet)
- [ ] T009 [US2] Write render tests for each of the 6 Tier 4 section headings in `internal/service/render/render_test.go`: a `SectionMap` with one non-empty Tier 4 slice must produce a heading line matching the authoritative string (e.g., `"SPEAKING ENGAGEMENTS\n"`)
- [ ] T010 [US2] Write empty-slice no-op tests in `internal/service/render/render_test.go`: a `SectionMap` with empty Tier 4 slices must produce no heading line for any Tier 4 section

### Implementation for User Story 2

- [ ] T011 [US2] Add `sectionWriter` type and the ordered `[]sectionWriter` registry slice to `internal/service/render/render.go`; replace the 10 hardcoded `writeX(...)` calls in `Render` with a range loop over the registry
- [ ] T012 [US2] Add 6 Tier 4 writer functions (`writeLanguages`, `writeSpeaking`, `writeOpenSource`, `writePatents`, `writeInterests`, `writeReferences`) to `internal/service/render/render.go` and append their entries to the registry slice; each writer must guard `len(items) == 0` and return immediately if empty (requires T005 complete — Tier 4 `SectionMap` fields must exist before writers can reference them)

**Checkpoint**: US2 complete — `go test -race ./internal/service/render/...` fully green; dual-render test passes; no hardcoded section-name strings in `Render`'s dispatch loop.

---

## Phase 5: User Story 3 — Binary-Ready Extractor Interface (Priority: P3)

**Goal**: Change `port.Extractor.Extract` from `(text string)` to `(data []byte)`. Update stub and 4 call sites. This phase is independent of US1 and US2 and may start immediately after Phase 1.

**Independent Test**: `go build ./... && go vet ./...` clean; `go test -race ./internal/service/extract/...` passes with the new signature.

### Tests for User Story 3 (write first — must FAIL before implementation) ⚠️

- [ ] T013 [US3] Update existing `Extract` call sites in `internal/service/extract/extract_test.go` to pass `[]byte(...)` arguments — tests must FAIL to compile before the interface is updated

### Implementation for User Story 3

- [ ] T014 [US3] Update `port.Extractor` interface to `Extract(data []byte) (string, error)` in `internal/port/extract.go`
- [ ] T015 [P] [US3] Update stub implementation signature to `func (s *Service) Extract(data []byte) (string, error) { return string(data), nil }` in `internal/service/extract/extract.go`
- [ ] T016 [P] [US3] Update 2 call sites in `internal/mcpserver/session_tools.go` to `extractSvc.Extract([]byte(rendered))` and `extractSvc.Extract([]byte(rawText))` at lines ~717 and ~735

**Checkpoint**: US3 complete — `go build ./... && go vet ./... && go test -race ./internal/service/extract/...` all green.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Full pipeline verification across all three user stories.

- [ ] T017 Run `go test -race ./internal/model/... ./internal/service/render/... ./internal/service/extract/... ./internal/mcpserver/...` — all green
- [ ] T018 [P] Run `go vet ./... && golangci-lint run` — zero warnings
- [ ] T019 Run quickstart.md end-to-end: onboard a resume with all 6 Tier 4 sections via `mcp__go-apply__add_resume`, run `mcp__go-apply__preview_ats_extraction`, confirm all Tier 4 headings appear in `extracted_text`
- [ ] T020 [P] Run quickstart.md regression check: onboard a resume with NO Tier 4 sections, confirm render output matches pre-spec behaviour (SC-006)
- [ ] T021 [P] Add a Tier 4 `preview_ats_extraction` test case to `internal/mcpserver/session_tools_test.go` using the existing test pattern: onboard a resume with at least one Tier 4 section (e.g., `Languages`) and assert the section heading appears in `extracted_text` (closes SC-002 automated coverage gap)

**Checkpoint**: All phases complete — spec ready for PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 3 (US1)**: Depends on Phase 1 baseline
- **Phase 4 (US2)**: Depends on Phase 3 completion (needs new `SectionMap` fields for Tier 4 writers)
- **Phase 5 (US3)**: Depends only on Phase 1 — fully independent; may be done in parallel with Phase 3 if desired
- **Phase 6 (Polish)**: Depends on Phases 3, 4, 5 all complete

### User Story Dependencies

| Story | Depends On | Notes |
|---|---|---|
| US1 (P1) | Phase 1 only | No story dependencies |
| US2 (P2) | US1 complete | Needs new `SectionMap` fields for Tier 4 writers; no other dependency |
| US3 (P3) | Phase 1 only | Completely independent — different files from US1 and US2 |

### Within Each Story

1. Tests written first → confirmed FAILING
2. Implementation tasks run
3. Tests confirmed PASSING
4. Story checkpoint verified before moving on

### Parallel Opportunities

**Within US1 (after T001)**:
```
T002 (resume_test.go) ──┐
T003 (resume_validate_test.go) ──┤── all 3 tests in parallel
T004 (onboard_sections_test.go) ─┘

T006 (resume_validate.go) ──┐ both after T005
T007 (onboard.go) ──────────┘
```

**US3 vs US1**: fully parallel (no shared files):
```
Phase 3 (US1: model/  + mcpserver/onboard.go)
Phase 5 (US3: port/   + extract/  + session_tools.go)
```

---

## Parallel Execution Example: US1 Tests

```bash
# Launch all 3 US1 test tasks together (all touch different files):
Task: "Add JSON round-trip tests for 6 entry structs in internal/model/resume_test.go"
Task: "Add knownSections tests in internal/model/resume_validate_test.go"
Task: "Add parseSectionsArg tests in internal/mcpserver/onboard_sections_test.go"
```

---

## Implementation Strategy

### MVP First (US1 Only)

1. Complete T001 (baseline)
2. Complete Phase 3 (T002–T007)
3. **STOP and VALIDATE**: `go test -race ./internal/model/... ./internal/mcpserver/...` green
4. US1 value delivered independently

### Incremental Delivery

1. T001 → US1 → US2 → US3 (sequential, single developer)
2. OR: T001 → US1 + US3 in parallel → US2
3. Polish after all stories complete

### Total Task Count: 20 tasks

| Phase | Tasks | Stories |
|---|---|---|
| Phase 1: Setup | 1 | — |
| Phase 3: US1 | 6 | US1 |
| Phase 4: US2 | 5 | US2 |
| Phase 5: US3 | 4 | US3 |
| Phase 6: Polish | 4 | — |

---

## Notes

- `[P]` tasks touch different files — safe to run in parallel
- Each story phase has an explicit checkpoint with a `go test` command
- SC-006 dual-render test (T008) lives alongside the implementation and can be removed in a follow-up once the registry is the sole path
- `open_source` uses underscore in all JSON tags, YAML tags, and `knownSections` keys — do not use hyphen or camelCase
- `ReferenceEntry{Name: "Available upon request"}` is valid — no per-field validation on Tier 4 entries
