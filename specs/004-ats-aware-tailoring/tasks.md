---
description: "Task list for feature 004 — ATS-Aware Resume Tailoring"
---

# Tasks: ATS-Aware Resume Tailoring

**Input**: Design documents from `/specs/004-ats-aware-tailoring/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/mcp-tools.md](./contracts/mcp-tools.md), [contracts/ports.md](./contracts/ports.md), [quickstart.md](./quickstart.md)

**Tests**: REQUIRED — Principle II (Test-First Development) is NON-NEGOTIABLE. Every implementation task is preceded by a failing test task in the same phase.

**Organization**: Tasks grouped by user story (US1–US4 in scope; US5–US8 deferred). Each story is independently deployable.

## Format: `- [ ] T### [P?] [USn?] (model) Description with file path`

- **[P]**: Parallelizable — touches a distinct file with no incomplete dependencies.
- **[USn]**: Binds task to a user story (US1–US4). Setup/Foundational/Polish phases carry no story label.
- **(model)**: Subagent model — `haiku` for mechanical/single-file work, `sonnet+opus` for ambiguous, multi-file, or design-sensitive work (sonnet executes, opus advises). Dispatch per `.claude/CLAUDE.md` parallel-subagent workflow.

## Subagent Dispatch Policy

Per user's model-selection rule (memory: `feedback_model_selection.md`, `feedback_parallel_agents.md`):
- **`haiku`**: deterministic changes — type definitions, enum/const blocks, map literals, deletions, compile-time guards, single-tool registrations.
- **`sonnet+opus`**: ambiguous work — Renderer YoE order logic, `ApplyEdits` op semantics, `ParseSections` prompt design, cross-mode migration paths, MCP envelope diffs that break existing callers. Opus acts as advisor (invoke `advisor()` before the first substantive edit and before declaring done).

Dispatch all `[P]`-tagged tasks inside a phase as **worktree-isolated parallel subagents**. Serial (non-`[P]`) tasks within a phase run after their dependencies clear.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Prep directory skeleton and regression-test ground truth.

- [ ] T001 [P] (haiku) Create package directories `internal/service/render/`, `internal/service/extract/` with `doc.go` stubs declaring package comments.
- [ ] T002 [P] (haiku) Record pre-feature test baseline: `go test ./... > /tmp/pre-004-baseline.txt` so any regression in untouched packages surfaces in diff.

**Checkpoint**: Directories exist; baseline captured.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Types, ports, validators that every user story imports. No user-facing behavior yet.

**⚠️ CRITICAL**: No user story work begins until Phase 2 completes and compiles.

### Tests (write first, must FAIL)

- [ ] T003 [P] (sonnet+opus) Write table-driven failing tests for `model.ValidateSectionMap` in `internal/model/resume_validate_test.go` covering: missing `contact.name`, unknown section key, schema version mismatch, skills kind/field mismatch, malformed date strings. Reference: data-model.md §SectionMap invariants, research §R4.
- [ ] T004 [P] (haiku) Write failing tests for `model.ExperienceEntry.BulletID(i,j)` format in `internal/model/resume_test.go` — expect exact format `exp-<i>-b<j>`.

### Model types (parallel)

- [ ] T005 [P] (haiku) Add `SectionMap`, `ContactInfo`, `ExperienceEntry`, `EducationEntry`, `ProjectEntry`, `CertificationEntry`, `AwardEntry`, `VolunteerEntry`, `PublicationEntry`, `ResumeRecord`, `CurrentSchemaVersion = 1` to `internal/model/resume.go` per data-model.md §SectionMap–§Resume.
- [ ] T006 [P] (haiku) Add `SkillsSection` + `SkillsKind` discriminated union to `internal/model/resume.go` per data-model.md §SkillsSection.
- [ ] T007 [P] (haiku) Add error sentinels `ErrSectionsMissing`, `ErrSchemaVersionUnsupported`, `ErrNotSupportedInMCPMode` and `SchemaError{Field,Reason}` to `internal/model/errors.go`.
- [ ] T008 [sonnet+opus] Implement `ValidateSectionMap` in `internal/model/resume_validate.go` per data-model.md invariants (required keys, unknown key rejection, SkillsKind/field consistency, date parsing for ISO `YYYY`/`YYYY-MM`/`Present`). Make T003 pass.
- [ ] T009 [P] (haiku) Implement `ExperienceEntry.ID(i) string` and `BulletID(i,j) string` in `internal/model/resume.go`. Make T004 pass.

### Port interfaces (parallel, depend only on model types)

- [ ] T010 [P] (haiku) Create `internal/port/render.go` with `Renderer` interface per contracts/ports.md §1.
- [ ] T011 [P] (haiku) Create `internal/port/extract.go` with `Extractor` interface per contracts/ports.md §2.
- [ ] T012 [P] (haiku) Extend `internal/port/tailor.go` — add `EditOp`, `Edit`, `EditRejection`, `EditResult`, `Tailor.ApplyEdits`. Keep existing `TailorResume` method for migration per contracts/ports.md §5.
- [ ] T013 [P] (haiku) Extend `internal/port/orchestrator.go` — add `Orchestrator.ParseSections(ctx, raw) (SectionMap, error)` per contracts/ports.md §3.
- [ ] T014 [P] (haiku) Extend `internal/port/resume.go` — add `ResumeRepository.LoadSections(label)` and `SaveSections(label, sections)` per contracts/ports.md §4.

**Checkpoint**: `go build ./internal/model/... ./internal/port/...` succeeds. `go test ./internal/model/...` green.

---

## Phase 3: User Story 1 — Tailoring works regardless of resume heading style (Priority: P1) 🎯 MVP

**Goal**: Deterministically edit sections by JSON key — no regex heading detection. Skills section with any heading (including "Skills & Abilities" / "Technical Stack" / absent) accepts edits.

**Independent Test**: Onboard a resume whose skills heading is "Skills & Abilities", submit a `replace` edit on section `"skills"`, confirm the edit applies and is echoed in `edits_applied`. Re-run with heading "Technical Stack" and with no heading at all — all three scenarios produce applied edits.

### Tests for US1 (write first, must FAIL)

- [ ] T015 [P] [US1] (sonnet+opus) Write failing unit tests for `tailor.Service.ApplyEdits` in `internal/service/tailor/apply_edits_test.go` — full matrix from research §R6: skills flat add/remove/replace, skills categorized add/remove/replace (with `<category>/<token>` target), experience-entry add/replace, bullet add/remove/replace with `exp-<i>-b<j>` target, summary replace, unknown section rejection, unknown op rejection, malformed bullet-id rejection, out-of-range target rejection, **absent-section targeting (key not in SectionMap) rejected distinctly from empty-section targeting (key present, value nil/empty slice) per FR-010**.
- [ ] T016 [P] [US1] (sonnet+opus) Write failing FS sidecar tests in `internal/repository/fs/resume_sections_test.go` — round-trip save/load, atomic rename under simulated crash (temp file leftover), ENOENT returns `ErrSectionsMissing`, schema-version mismatch returns `ErrSchemaVersionUnsupported`.
- [ ] T017 [P] [US1] (sonnet) Write failing MCP contract test for unified edit envelope in `internal/mcpserver/tailor_tools_test.go` — T1 and T2 both accept `{edits:[{section,op,target?,value?}]}`; verify `edits_applied`/`edits_rejected` shape per contracts/mcp-tools.md §4.
- [ ] T018 [P] [US1] (sonnet) Write failing MCP contract test for `onboard_user` + `add_resume` accepting `sections` field and rejecting malformed payloads with typed errors `missing_sections` / `invalid_sections` per contracts/mcp-tools.md §1–2.

### Implementation for US1

- [ ] T019 [P] [US1] (sonnet+opus) Implement `tailor.Service.ApplyEdits` in `internal/service/tailor/apply_edits.go` — stateless, deep-copies input, iterates edits, applies per-op semantics, collects rejections. Compile-time guard `var _ port.Tailor = (*Service)(nil)`. Make T015 pass.
- [ ] T020 [P] [US1] (sonnet+opus) Implement FS sidecar in `internal/repository/fs/resume.go` — add `LoadSections`/`SaveSections` reading/writing `<dataDir>/inputs/<label>.sections.json` via temp+rename atomic write; validate on load. Make T016 pass.
- [ ] T021 [US1] (sonnet) Wire unified edits envelope into `internal/mcpserver/tailor_tools.go` — T1 and T2 both decode `edits[]`, call `Tailor.ApplyEdits`, return `{previous_score, new_score, edits_applied, edits_rejected, sections, schema_version}` per contracts §4. Enforce per-tier caps (`MaxTier1SkillRewrites`, `MaxTier2BulletRewrites`) returning `too_many_edits`. Depends on T019. Make T017 pass.
- [ ] T022 [US1] (sonnet) Extend `internal/mcpserver/onboard.go` — `onboard_user` and `add_resume` accept `sections` JSON, validate via `ValidateSectionMap`, persist via `SaveSections`. Emit typed errors per contracts §1–2. Depends on T020. Make T018 pass.
- [ ] T023 [US1] (haiku) DELETE `skillsHeaderRe` (`internal/service/tailor/tier1.go:11`), `ExtractSkillsSection`, `AddKeywordsToSkillsSection`, `ApplySkillsRewrites` (`internal/service/tailor/mechanical.go`), `skillsFooterRe`. Remove the deprecated `TailorResume` call sites that referenced them. Depends on T021 — compile must still pass because callers were migrated there.
- [ ] T024 [US1] (sonnet+opus) Integration test `internal/mcpserver/e2e_us1_test.go` — replay: add_resume with "Skills & Abilities" content in `sections.skills.categorized`, submit T1 with skill replace, assert `edits_applied=1`, `edits_rejected=[]`, final `sections.skills` reflects the change. Tag `//go:build integration`.

**Checkpoint**: `go test ./internal/service/tailor/... ./internal/repository/fs/... ./internal/mcpserver/... -run ".*US1|.*TailorTool|.*Onboard"` green. Regression targets deleted. `go vet ./...` clean.

---

## Phase 4: User Story 2 — Synonyms don't penalize candidates (Priority: P1)

**Goal**: Bidirectional alias matching (Apache Spark↔PySpark, PostgreSQL↔Postgres, Kubernetes↔K8s, JavaScript↔JS, TypeScript↔TS). No false positives on `C++`, `.NET`, `Go`.

**Independent Test**: JD requires "Apache Spark"; resume lists only "PySpark". `submit_keywords` returns Apache Spark in `req_matched`. Reverse pairing also matches. `Go` in resume does not match a JD requiring `JavaScript`.

### Tests for US2 (write first, must FAIL)

- [ ] T025 [P] [US2] (sonnet+opus) Write failing tests for `scorer.AliasSet` in `internal/service/scorer/aliases_test.go` — bidirectional expansion for all five pairs, case-insensitive lookup, constructor builds reverse index, disjoint invariant (no alias collides with a canonical of a different cluster), `Expand(x)[0] == x`.
- [ ] T026 [P] [US2] (sonnet+opus) Write failing integration tests for `classify` in `internal/service/scorer/scorer_test.go` — regression cases: `C++`, `.NET`, `Go` remain word-boundary-correct (no false positives); positive alias cases credit the canonical keyword exactly once even when resume lists both forms.

### Implementation for US2

- [ ] T027 [P] [US2] (haiku) Create `internal/service/scorer/aliases.go` with `AliasSet` struct, `NewDefaultAliasSet()` seeded with the five pairs (Apache Spark↔PySpark, PostgreSQL↔Postgres, Kubernetes↔K8s, JavaScript↔JS, TypeScript↔TS), `Expand(keyword) []string` with O(1) reverse-index lookup per data-model.md §AliasSet. Make T025 pass.
- [ ] T028 [US2] (sonnet+opus) Modify `classify` at `internal/service/scorer/scorer.go:199-213` to OR-match each keyword against `AliasSet.Expand`, preserving existing `compileKeywordPattern` word-boundary behavior. Ensure no-double-count when both forms present. Depends on T027. Make T026 pass.

**Checkpoint**: `go test ./internal/service/scorer/...` green including all regression cases.

---

## Phase 5: User Story 3 — Orchestrator sees the full structured resume (Priority: P2)

**Goal**: `submit_keywords` response includes full `sections` payload and `schema_version`. `skills_section` field removed. Pre-feature records migrate per mode.

**Independent Test**: After scoring, response includes `sections` reflecting the best resume's structured content. Deleting the sidecar and rerunning MCP path returns `sections_missing` with `raw` text; Headless path auto-parses transparently.

### Tests for US3 (write first, must FAIL)

- [ ] T029 [P] [US3] (sonnet+opus) Write failing `Orchestrator.ParseSections` contract tests in `internal/service/orchestrator/parse_sections_test.go` — valid raw → valid SectionMap; malformed LLM response wrapped as `SchemaError`; Unicode / multi-line resumes; no retry on validation failure.
- [ ] T030 [P] [US3] (sonnet) Write failing `submit_keywords` envelope test in `internal/mcpserver/session_tools_test.go` — response shape matches contracts §3 (sections, schema_version present; skills_section absent).
- [ ] T031 [P] [US3] (sonnet+opus) Write failing migration tests in `internal/mcpserver/migration_test.go` — pre-feature record (txt only, no sidecar) → MCP `submit_keywords` returns `sections_missing` with `raw`; Headless `submit_keywords` auto-parses + persists.

### Implementation for US3

- [ ] T032 [P] [US3] (sonnet+opus) Implement `orchestrator.Service.ParseSections` in `internal/service/orchestrator/parse_sections.go` — JSON-schema-constrained system prompt per research §R2, calls `LLMClient.ChatComplete`, decodes, runs `ValidateSectionMap`, wraps errors with `fmt.Errorf("parse sections: %w", err)`. No retry. Make T029 pass.
- [ ] T033 [P] [US3] (haiku) Add MCP stub `parseSectionsStub` returning `ErrNotSupportedInMCPMode` from the MCP-mode Orchestrator adapter per contracts/ports.md §3.
- [ ] T034 [US3] (sonnet+opus) Modify `internal/mcpserver/session_tools.go` `submit_keywords` handler — pull sections for best resume, include in response, drop `skills_section`. On `ErrSectionsMissing`: MCP mode returns `sections_missing` envelope with `raw`; Headless invokes `Orchestrator.ParseSections`, persists, retries transparently. Make T030, T031 pass.
- [ ] T035 [P] [US3] (sonnet) Integration test `internal/mcpserver/e2e_us3_test.go` replays both migration flows end-to-end. Tag `//go:build integration`.

**Checkpoint**: `go test ./internal/service/orchestrator/... ./internal/mcpserver/...` green. `submit_keywords` breaking envelope change is contract-tested.

---

## Phase 6: User Story 4 — Architectural seam ready for ATS-accurate scoring (Priority: P2)

**Goal**: `Renderer` + `Extractor` interfaces with identity default impls. YoE-driven default order; orchestrator `order` override honored. Canonical labels emitted. New `preview_ats_extraction` MCP tool.

**Independent Test**: `Render(sections)` produces Experienced-tier order for ≥3 YoE and Entry-level tier for <3; with `sections.Order` set, uses it verbatim. Headings match the canonical label set. `preview_ats_extraction` returns identity output.

### Tests for US4 (write first, must FAIL)

- [ ] T036 [P] [US4] (sonnet+opus) Write failing tests for `render.Service.Render` in `internal/service/render/render_test.go` — YoE ≥3 experienced tier order; YoE <3 entry-level tier order; empty `experience` triggers entry-level; explicit `Order` override used verbatim; canonical labels emitted regardless of input label; empty sections omitted; missing contact name returns error. Reference: research §R1, §R7.
- [ ] T037 [P] [US4] (haiku) Write failing tests for `extract.IdentityService.Extract` in `internal/service/extract/extract_test.go` — `Extract(s) == s` for arbitrary strings including empty and Unicode.
- [ ] T038 [P] [US4] (sonnet) Write failing MCP contract test for `preview_ats_extraction` in `internal/mcpserver/preview_test.go` per contracts §5.

### Implementation for US4

- [ ] T039 [US4] (sonnet+opus) Implement `render.Service.Render` in `internal/service/render/render.go` — compute YoE by merging `ExperienceEntry` date intervals (research §R1), pick tier, honor `sections.Order` override, emit canonical labels (map from research §R7), render skills flat-or-categorized, omit empty sections, return error on missing contact name. Compile-time guard `var _ port.Renderer = (*Service)(nil)`. Make T036 pass.
- [ ] T040 [P] [US4] (haiku) Implement `extract.IdentityService.Extract` in `internal/service/extract/extract.go` — return input verbatim. Compile-time guard. Make T037 pass.
- [ ] T041 [P] [US4] (haiku) Create `internal/mcpserver/preview.go` registering `preview_ats_extraction` — call `Renderer.Render` then `Extractor.Extract` on session's best resume sections; emit envelope per contracts §5. Make T038 pass.
- [ ] T042 [US4] (sonnet) Wire scorer to consume `Extractor.Extract(Renderer.Render(sections))` rather than raw text in `internal/mcpserver/session_tools.go` scoring path. Depends on T039, T040.
- [ ] T043 [US4] (sonnet) Wire `finalize` handler to persist `Renderer.Render(session.sections)` as the tailored artifact in `internal/mcpserver/finalize.go` (or current finalize location). Depends on T039.

**Checkpoint**: `go test ./internal/service/render/... ./internal/service/extract/... ./internal/mcpserver/...` green. Quickstart §5–§6 scenarios runnable.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [ ] T044 [P] (haiku) Remove deprecated `port.Tailor.TailorResume` method and its headless call sites — no backwards-compat shim. Update compile-time guards.
- [ ] T045 [P] (haiku) Add structured-log entries (`slog.Info` with session_id, op, edit_count, outcome, elapsed_ms) to every new MCP handler: `onboard_user`, `add_resume`, `submit_keywords`, `submit_tailor_t1`, `submit_tailor_t2`, `preview_ats_extraction`. Per Principle V.
- [ ] T046 [P] (haiku) Add `logger.Verbose()` dumps for parsed SectionMap, pre/post-edit diff, alias expansions that credited a match. Per Principle V.
- [ ] T047 [P] (sonnet) Integration test `internal/mcpserver/e2e_playstation_test.go` replays quickstart §11 end-to-end: add_resume with "Skills & Abilities" sections, load_jd, submit_keywords (Apache Spark in req_matched via PySpark alias), submit_tailor_t1 with unified edits, score ≥70 or T2 graduation, `preview_ats_extraction` returns expected text.
- [ ] T048 (sonnet+opus) Coverage audit: run `go test -coverprofile=coverage.out ./... && go tool cover -func coverage.out` — verify ≥80% for `internal/model`, `internal/service/{render,extract,scorer,tailor,orchestrator}`, `internal/repository/fs`, `internal/mcpserver`. Backfill any gap before PR.
- [ ] T049 (sonnet) Run `go vet ./...`, `go test -race ./...`, `go test -tags integration ./...` — all must pass. Compare `go test ./... > /tmp/post-004.txt` against `/tmp/pre-004-baseline.txt` — no unexpected regressions in untouched packages.
- [ ] T050 (haiku) Update `README.md` and any top-level user-facing doc to reference the new MCP tool `preview_ats_extraction` and the `sections` input in `add_resume`/`onboard_user`.

**Checkpoint**: All constitution gates pass. Feature ready for PR.

---

## Deferred Phases (US5–US8) — Not Implemented Now

These user stories are **specified but deferred**. Do NOT generate tasks for them in this feature; they will be picked up in a future spec iteration. Listed here so the scope boundary is explicit.

- **US5** (FR-D01–D03) — Real PDF renderer + `pdftotext`-based extractor + layout survival diff.
- **US6** (FR-D06) — Tier 4 schema keys (languages, speaking, open_source, patents, interests, references).
- **US7** (FR-D05) — User-configurable alias overrides (config-driven).
- **US8** (FR-D07) — ATS-specific extraction profiles (Workday, Greenhouse, Taleo, iCIMS).
- **Also deferred**: FR-D04 (multiple render templates), FR-D08 (per-section scoring weights).

---

## Dependencies & Execution Order

### Phase dependencies

```
Setup (P1) ──▶ Foundational (P2) ──┬─▶ US1 (P3) ──┐
                                   ├─▶ US2 (P4) ──┤
                                   ├─▶ US3 (P5) ──┼─▶ Polish (P7)
                                   └─▶ US4 (P6) ──┘
```

- US1–US4 may proceed **in parallel** once Phase 2 completes (each touches distinct files; US3 and US4 only overlap in `session_tools.go`, which is sequentialized by T034 → T042).
- Polish runs after all in-scope user stories merge.

### Intra-phase task dependencies (key blockers)

- T008 (ValidateSectionMap) blocks T020, T022 (FS + onboard use validation).
- T019 (ApplyEdits) blocks T021 (MCP tailor handler).
- T020 (Load/SaveSections) blocks T022, T034.
- T027 (AliasSet) blocks T028 (scorer integration).
- T032 (ParseSections) blocks T034 (Headless auto-parse path).
- T039 (Renderer) blocks T042, T043 (scorer wiring + finalize).
- T044 (remove `TailorResume`) runs last — depends on every US1 handler migration completing.

### Parallel opportunities (dispatch as worktree-isolated subagents)

**Phase 2 fan-out** (after T003/T004 tests written):
```
haiku:        T005, T006, T007, T009, T010, T011, T012, T013, T014   (9 in parallel)
sonnet+opus:  T008                                                      (serial, advisor-gated)
```

**US1 fan-out** (after T015–T018 tests written):
```
sonnet+opus:  T019, T020       (parallel, distinct files)
haiku:        T023              (parallel, distinct file — deletions)
sonnet:       T021 → T022       (serial; both edit MCP handlers in sequence)
sonnet+opus:  T024              (integration test, after T021/T022)
```

**US2 fan-out**:
```
haiku:        T027              (first, blocks T028)
sonnet+opus:  T028              (after T027)
```

**US3 fan-out**:
```
sonnet+opus:  T032              (parallel with T033)
haiku:        T033              (parallel with T032)
sonnet+opus:  T034 → T035       (serial; T034 touches handler, T035 is integration)
```

**US4 fan-out**:
```
sonnet+opus:  T039              (blocks T042, T043)
haiku:        T040, T041        (parallel with T039, no shared files)
sonnet:       T042, T043        (serial after T039)
```

**Polish fan-out**:
```
haiku:        T044, T045, T046, T050   (4 in parallel)
sonnet:       T047                     (parallel with haiku tasks)
sonnet+opus:  T048 → T049              (serial, at end)
```

---

## Parallel Example — Phase 2 Foundational

Dispatch as a single orchestration batch (worktree-isolated subagents per memory `feedback_parallel_agents.md`):

```
Wave 1 (tests, parallel):
  Agent(sonnet+opus): T003 — model.ValidateSectionMap failing tests
  Agent(haiku):       T004 — BulletID format failing test

Wave 2 (models + ports, parallel after Wave 1):
  Agent(haiku):       T005 — resume.go entity types
  Agent(haiku):       T006 — SkillsSection union
  Agent(haiku):       T007 — errors.go sentinels
  Agent(haiku):       T009 — ExperienceEntry.ID/BulletID
  Agent(haiku):       T010 — port/render.go
  Agent(haiku):       T011 — port/extract.go
  Agent(haiku):       T012 — port/tailor.go extension
  Agent(haiku):       T013 — port/orchestrator.go extension
  Agent(haiku):       T014 — port/resume.go extension

Wave 3 (validator — needs types):
  Agent(sonnet+opus): T008 — ValidateSectionMap impl; advisor() before start + before done
```

---

## Implementation Strategy

### MVP (US1 only)

1. Phase 1 → Phase 2 → Phase 3 (US1).
2. Stop and validate: replay "Skills & Abilities" T1 scenario (quickstart §1 + §3).
3. Open PR; merge; demo.

### Incremental delivery

1. MVP as above.
2. Add US2 (alias scoring) — replay quickstart §2; PR.
3. Add US3 (sections in response + migration) — replay quickstart §7; PR.
4. Add US4 (Renderer/Extractor seam + preview) — replay quickstart §5/§6/§8; PR.
5. Polish (Phase 7) — coverage, logs, README, final PlayStation replay.

### Model discipline

- **Haiku tasks are unsupervised** — dispatch and move on; review diff on return.
- **Sonnet tasks** — review diff before merging into the feature branch.
- **Sonnet+opus tasks** — sonnet executes; the subagent MUST call `advisor()` (opus) before the first substantive edit and before declaring done. This is load-bearing for: T008, T015, T016, T019, T020, T024, T025, T026, T028, T029, T031, T032, T034, T036, T039, T048.

---

## Validation

Every checklist item below MUST be satisfied before opening the feature PR:

- [ ] Every task has a checkbox, task ID, and file path (format audit on tasks.md itself).
- [ ] Every user-story task carries `[USn]`; setup/foundational/polish tasks do not.
- [ ] Every task has a model annotation (`haiku` or `sonnet+opus`; `sonnet` for non-ambiguous multi-file work).
- [ ] Every implementation task in US1–US4 is preceded by a failing test task in the same phase (Principle II).
- [ ] Parallel `[P]` tasks touch disjoint files.
- [ ] No task depends on a US5–US8 deliverable.
- [ ] `checklists/regression-fidelity.md` re-reviewed after tasks complete — every CHK item verifiable.

---

## Notes

- All file paths absolute from repo root.
- Branch: `004-ats-aware-tailoring`.
- Commit discipline: one commit per task (conventional commit: `feat(sections): …`, `test(sections): …`, `refactor(sections): …`); squash-merge PR to `dev`.
- Per `feedback_task_completion.md`: mark a task `completed` only when its PR is merged.
- Per `feedback_branch_cleanup.md`: delete worktree + local + remote branches after every PR merge.
