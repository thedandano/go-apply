# Tasks: T1 Skill Section Rewrites

**Input**: Design documents from `specs/003-t1-skill-rewrites/`
**Branch**: `003-t1-skill-rewrites`
**Plan**: [plan.md](plan.md) | **Spec**: [spec.md](spec.md)

**Tests**: Included — constitution principle II (Test-First) requires failing tests before every implementation task.

**Organization**: Grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no conflicting dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Exact file paths included in all descriptions

---

## Phase 1: Setup

**Purpose**: Confirm clean baseline before any changes land.

- [x] T001 Run `go test ./... -race` on branch `003-t1-skill-rewrites` and confirm all existing tests pass before touching any files

**Checkpoint**: All existing tests green — safe to begin Phase 2

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: `ExtractSkillsSection` (shared by US1 and US2) and `MaxTier1SkillRewrites` config (needed by US1 handler). Must be complete before any user story phase begins.

**⚠️ CRITICAL**: US1 and US2 cannot be implemented until T004 is done; the cap config (T003) must exist before T009.

- [x] T002 [P] Write failing unit tests for `ExtractSkillsSection` in `internal/service/tailor/tier1_test.go` — cover: Skills header detected (categorised section), Skills header detected (flat section), `start`/`end` line indices correct, `section` text verbatim including header line, `found=false` when no Skills header present
- [x] T003 [P] [US3] Add `MaxTier1SkillRewrites int` to `TailorDefaults` struct in `internal/config/defaults.go`; add `"max_tier1_skill_rewrites": 5` to `internal/config/defaults.json`
- [x] T004 Implement `ExtractSkillsSection(resumeText string) (section string, start, end int, found bool)` in `internal/service/tailor/tier1.go` — reuse existing `isSkillsHeaderLine`; scan lines to find header, advance to next section header or EOF for `end`; run T002 tests and confirm all pass (depends on T002)

**Checkpoint**: `ExtractSkillsSection` tests green; config field added — user story phases can now begin

---

## Phase 3: User Story 1 — Inline Replacement (Priority: P1) 🎯 MVP

**Goal**: `submit_tailor_t1` accepts `skill_rewrites` (array of `{original, replacement}` pairs) and applies them as string replacements scoped strictly to the Skills section.

**Independent Test**: POST `submit_tailor_t1` with one rewrite targeting an existing Skills line; verify the Skills section contains the replacement inline; verify text outside the Skills section is unchanged when `original` appears there too (US1 Scenario 2).

### Tests for User Story 1 ⚠️ Write first — must FAIL before T008/T009

- [x] T005 [P] [US1] Write failing unit tests for `ApplySkillsRewrites` in `internal/service/tailor/mechanical_test.go` — cover: replacement applied inside Skills section; text outside Skills section NOT modified when same string appears there; `substitutions_made` count (entry-level: 1 per matched pair, regardless of occurrence count); `skills_section_found=false` path returns original text unchanged; empty `original` entry skipped; all entries empty `original` → 0 count, section found=true (service-layer; handler validates and rejects before reaching service — see T006); ordered application (FR-007: array order)
- [x] T006 [P] [US1] Write failing handler tests for `HandleSubmitTailorT1WithConfig` in `internal/mcpserver/session_tools_test.go` — cover: missing `skill_rewrites` param → `missing_skill_rewrites`; invalid JSON → `invalid_skill_rewrites`; empty array → `empty_skill_rewrites`; all-empty-original entries after filtering → `empty_skill_rewrites`; len > MaxTier1SkillRewrites → `too_many_rewrites`; valid rewrites → `substitutions_made` int in response; `skills_section_found: false` when no Skills section; `previous_score` and `new_score` present
- [x] T007 [P] [US1] Update `internal/mcpserver/session_tools_retention_test.go` — replace all `skill_adds` JSON payloads with `skill_rewrites` format (`[{"original":"...","replacement":"..."}]`) so retention tests use the new interface

### Implementation for User Story 1

- [x] T008 [US1] Implement `ApplySkillsRewrites(resumeText string, rewrites []port.BulletRewrite) (string, int, bool)` in `internal/service/tailor/mechanical.go` — call `ExtractSkillsSection`; if not found return original, 0, false; iterate rewrites in array order, skip entries where `Original==""`, run `strings.ReplaceAll` on section substring for each rewrite, increment count if match; splice modified section back via start/end indices; return modified text, count, true (depends on T005, T004)
- [x] T009 [US1] Update `HandleSubmitTailorT1WithConfig` in `internal/mcpserver/session_tools.go` — rename `t1Data.AddedKeywords []string` → `SubstitutionsMade int`; parse `skill_rewrites` param as `[]port.BulletRewrite`; add validation gates in order: missing/empty-string → `missing_skill_rewrites`, parse error → `invalid_skill_rewrites`, len==0 → `empty_skill_rewrites`, all-entries-empty-original → `empty_skill_rewrites`, len > cfg.Defaults.Tailor.MaxTier1SkillRewrites → `too_many_rewrites`; call `tailor.ApplySkillsRewrites`; populate `SubstitutionsMade` and `SkillsSectionFound` in response; add structured `slog` entry logging operation name, `substitutions_made`, `skills_section_found`, and session ID (Constitution V) (depends on T006, T008, T003)

**Checkpoint**: US1 independently testable — T1 end-to-end flow works with skill_rewrites; all retention tests green

---

## Phase 4: User Story 2 — skills_section in submit_keywords (Priority: P2)

**Goal**: `submit_keywords` response includes `skills_section` field with the verbatim Skills section text of the best-scored resume, enabling the orchestrator to write accurate `{original, replacement}` pairs.

**Independent Test**: Call `submit_keywords` against a session with a resume that has a Skills section; verify `skills_section` field is present and matches the verbatim section text. Call again with a resume with no Skills header; verify field is absent from response.

### Tests for User Story 2 ⚠️ Write first — must FAIL before T011

- [x] T010 [US2] Write failing handler tests for `HandleSubmitKeywordsWithConfig` in `internal/mcpserver/session_tools_test.go` — cover: response includes `skills_section` field with verbatim Skills section text when best resume has a Skills header; `skills_section` absent from response (`omitempty`) when best resume has no Skills section header

### Implementation for User Story 2

- [x] T011 [US2] In `internal/mcpserver/session_tools.go`: add `SkillsSection string \`json:"skills_section,omitempty"\`` to `submitKeywordsData` struct; in `HandleSubmitKeywordsWithConfig` after scoring, call `loadBestResumeText` then `tailor.ExtractSkillsSection`; if `found`, populate `SkillsSection` with extracted section text; add structured `slog` entry logging `skills_section` extraction result (found/not-found) and session ID (Constitution V) (depends on T010, T004)

**Checkpoint**: US1 + US2 independently functional — submit_keywords returns Skills section context; T1 uses it to make precise rewrites

---

## Phase 5: Polish & Cross-Cutting Concerns (US3 coverage)

**Purpose**: Prompt and tool description updates (FR-006/FR-007), US3 cap independent verification, and final quality gates.

- [x] T012 [P] Update `submit_tailor_t1` tool description in `internal/mcpserver/server.go` — replace `skill_adds` with `skill_rewrites`; document `{original, replacement}` shape; reference `MaxTier1SkillRewrites` cap
- [x] T013a [P] Write failing test in `internal/mcpserver/prompt_test.go` asserting the workflow prompt string contains "prefer one-for-one" substring — must FAIL before T013 (Constitution II: Test-First for FR-006 testable prompt content requirement)
- [x] T013 Update all 5 `skill_adds` references to `skill_rewrites` in `internal/mcpserver/prompt.go`; add explicit "prefer one-for-one swaps" language and conciseness guidance (≤40 chars extension per line for single-page resumes); verify resulting text is grep-able for "prefer one-for-one" (FR-006 testability requirement from CHK033)
- [x] T014 Run `go test ./... -race -coverprofile=coverage.out` and confirm ≥80% coverage gate passes; run `go tool cover -html=coverage.out` to spot any uncovered paths in `ApplySkillsRewrites` or `ExtractSkillsSection`
- [x] T015 Run `go vet ./...` and `golangci-lint run`; fix any issues before opening PR

**Checkpoint**: All gates green — ready for PR

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — BLOCKS all user story phases
- **Phase 3 (US1)**: Depends on Phase 2 (needs T004 for ApplySkillsRewrites, T003 for cap check)
- **Phase 4 (US2)**: Depends on Phase 2 (needs T004 for ExtractSkillsSection call in handler); independent of Phase 3
- **Phase 5 (Polish)**: Depends on Phases 3 and 4 both complete

### User Story Dependencies

- **US1 (P1)**: Requires Phase 2 complete — no dependency on US2 or US3
- **US2 (P2)**: Requires Phase 2 complete — no dependency on US1 (different handler, different struct field)
- **US3 (P3)**: Config (T003) is Foundational; cap validation is implemented in T009 (US1); prompt updates are Phase 5

### Within Phase 3 (US1)

- T005, T006, T007 can run in parallel (different files, all test-writing tasks)
- T008 depends on T005 (failing tests) and T004 (ExtractSkillsSection available)
- T009 depends on T006 (failing tests), T008 (ApplySkillsRewrites), T003 (config field)
- T007 can run any time after T009's interface is defined (interface is already in the plan)

---

## Parallel Opportunities

### Phase 2 — run T002 and T003 simultaneously

```
Task: "Write failing ExtractSkillsSection tests in internal/service/tailor/tier1_test.go"
Task: "Add MaxTier1SkillRewrites to internal/config/defaults.go + defaults.json"
```

### Phase 3 — run T005, T006, T007 simultaneously

```
Task: "Write failing ApplySkillsRewrites tests in internal/service/tailor/mechanical_test.go"
Task: "Write failing T1 handler tests in internal/mcpserver/session_tools_test.go"
Task: "Update skill_adds payloads in internal/mcpserver/session_tools_retention_test.go"
```

### Phase 4+5 — US2 and Phase 5 preparation can overlap

Once US1 (Phase 3) is complete, US2 (Phase 4) and Phase 5 tasks T012/T013 can begin in parallel:

```
Task: "Write failing submit_keywords skills_section tests in session_tools_test.go" [T010]
Task: "Update server.go tool description" [T012]
Task: "Update prompt.go 5 sites" [T013]
```

---

## Implementation Strategy

### MVP First (US1 Only — Phase 1 + 2 + 3)

1. Phase 1: Confirm baseline green
2. Phase 2: `ExtractSkillsSection` + config → foundational ready
3. Phase 3: `ApplySkillsRewrites` + T1 handler → **T1 skill-rewrite works end-to-end**
4. **STOP and VALIDATE**: Run the T1 handler against a real resume; confirm no bare-keyword append; confirm text outside Skills section unchanged
5. Open draft PR for US1 if validated

### Incremental Delivery

1. Setup + Foundational → ExtractSkillsSection available, config loaded
2. US1 complete → T1 uses skill_rewrites, no more append artifact (MVP shipped)
3. US2 complete → submit_keywords returns skills_section (orchestrator can write accurate rewrites)
4. Polish complete → prompts updated, coverage gate passed, PR ready

### Single-developer sequence

Phase 1 → Phase 2 (T002 + T003 parallel, then T004) → Phase 3 (T005 + T006 + T007 parallel, then T008, then T009) → Phase 4 (T010, T011) → Phase 5 (T012 + T013, then T014, T015)

---

## Notes

- `[P]` tasks touch different files and have no cross-dependency — safe to launch as parallel subagents
- Test tasks use the `sonnet` model; mechanical/config changes use `haiku` (per CLAUDE.md agent dispatch guidance)
- Each phase ends at a working, independently testable state
- Retention test update (T007) can be dispatched as a `haiku` mechanical task — it is a pure find-and-replace of payload format
- Constitution principle IV (No Silent Failures): the 5 validation gates in T009 must be implemented in order; do not collapse or short-circuit them
- FR-006 testability (CHK033): after T013, `grep -c "prefer one-for-one" internal/mcpserver/prompt.go` must return ≥ 1
