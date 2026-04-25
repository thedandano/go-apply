# Tasks: T1 Category-Aware Skills Edits

**Input**: Design documents from `specs/005-fix-t1-categorized-skills/`
**Prerequisites**: plan.md ✓, spec.md ✓, data-model.md ✓, contracts/submit_tailor_t1.md ✓, research.md ✓

**Agent Dispatch**: Agent A (Sonnet) owns port + service layer; Agent B (Haiku) owns MCP adapter. Both run in parallel after Phase 1.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel with other [P] tasks in the same phase
- **[Story]**: User story — [US1], [US2], [US3]

---

## Phase 1: Foundational (Blocking Prerequisite)

**Purpose**: `Category` field on `port.Edit` must exist before service routing or MCP description changes can compile. Included in Agent A's worktree scope.

**⚠️ CRITICAL**: This single task unblocks both Agent A and Agent B.

- [ ] T001 Add `Category string \`json:"category,omitempty"\`` to the `Edit` struct in `internal/port/tailor.go`

**Checkpoint**: `go build ./...` passes — Agents A and B can be dispatched in parallel.

---

## Phase 2: Service Layer — Agent A (Sonnet) [US1 + US2]

**Goal**: `applySkillsEdit` routes category-targeted ops to the correct `map[string][]string` entry, rejects missing/unknown categories with actionable messages, and preserves the existing flat path unchanged.

**Agent**: Sonnet — non-trivial logic, TDD required
**Worktree files**: `internal/port/tailor.go`, `internal/service/tailor/apply_edits.go`, `internal/service/tailor/apply_edits_test.go`

**Independent Test**: `go test ./internal/service/tailor/... -v -run "skills categorized"` — all new tests pass; all existing flat-skills tests pass.

### Tests (TDD — write first, verify they FAIL, then implement)

- [ ] T002 [US1] [US2] In `internal/service/tailor/apply_edits_test.go`: rename existing test `"skills categorized kind rejects flat ops"` → `"skills categorized rejects ops with missing category"` and update assertion to check rejection reason contains both `"requires a category"` and `"available:"` (keep structural assertions: `Categorized["Cloud"]` len==2, `Flat==""`)
- [ ] T003 [P] [US2] In `internal/service/tailor/apply_edits_test.go`: add test `"skills categorized rejects ops with unknown category"` — input `Category: "Nonexistent"` on categorized skills; assert reason contains `"category \"Nonexistent\" not found"` and `"available:"`; assert no category contents changed
- [ ] T004 [P] [US1] In `internal/service/tailor/apply_edits_test.go`: add test `"skills categorized add appends items to named category"` — valid category, `op=add`, `value="Apache Kafka, Spark"`; assert both items appended individually to the category list AND `Kind=="categorized"` AND `Flat==""` after edit
- [ ] T005 [P] [US1] In `internal/service/tailor/apply_edits_test.go`: add test `"skills categorized replace sets named category"` — valid category, `op=replace`; assert category list is fully replaced with parsed items AND `Kind=="categorized"` AND `Flat==""` after edit
- [ ] T006 [P] [US1] In `internal/service/tailor/apply_edits_test.go`: add test `"skills categorized add with comma separated value splits items"` — assert `value="AWS, GCP"` produces two separate items, not one string entry
- [ ] T007 [P] [US2] In `internal/service/tailor/apply_edits_test.go`: add test `"skills categorized empty map rejection lists no categories"` — empty `Categorized` map; assert rejection reason includes `"available:"` with an empty or zero-entry list
- [ ] T008 [P] [US1] In `internal/service/tailor/apply_edits_test.go`: add test `"skills flat ignores category field"` — flat skills section with `Category` set on the edit; assert edit applied normally to the flat string
- [ ] T008b [P] [US2] In `internal/service/tailor/apply_edits_test.go`: add test `"skills categorized mixed call applies valid and rejects invalid"` — two edits in one call: first has valid `category`, second has unknown `category: "Nonexistent"`; assert `EditsApplied==1`, `EditsRejected==1`, valid edit's category contents updated, second edit has rejection reason containing `"not found"` and `"available:"`
- [ ] T009 [US1] [US2] Run `go test ./internal/service/tailor/... -run "skills categorized"` and confirm it **FAILS** (red phase confirmed before implementing)

### Implementation

- [ ] T010 [P] [US1] [US2] In `internal/service/tailor/apply_edits.go`: add `sortedKeys(m map[string][]string) string` — returns sorted, comma-joined category names
- [ ] T011 [P] [US1] [US2] In `internal/service/tailor/apply_edits.go`: add `splitTrim(s string) []string` — splits on `,`, trims whitespace, drops empty tokens
- [ ] T012 [US1] [US2] In `internal/service/tailor/apply_edits.go`: replace the categorized guard block in `applySkillsEdit` with category routing logic per `data-model.md §applySkillsEdit` — missing category → FR-003 message; unknown category → FR-004 message; `add` → splitTrim+append; `replace` → splitTrim+assign; unsupported op → existing error
- [ ] T013 [US1] [US2] Run `go test ./... -race` and confirm: all new categorized tests pass; all existing flat-skills and experience tests pass; pre-commit coverage gate (≥80%) passes

**Checkpoint**: Agent A work complete — open PR targeting `dev`. All tests green.

---

## Phase 3: MCP Adapter — Agent B (Haiku) [US3]

**Goal**: The `submit_tailor_t1` tool description and workflow prompt teach the orchestrator to include `category` when the resume has a categorized skills section.

**Agent**: Haiku — mechanical string updates, no logic changes
**Worktree files**: `internal/mcpserver/server.go`, `internal/mcpserver/prompt.go`
**Can run in parallel with Phase 2** — no file overlap with Agent A.

**Independent Test**: Read `server.go` and confirm `edits` description contains `"category"` and `"categorized"`. Read `prompt.go` and confirm `workflowPromptText` contains `"category?"` and `"skills_section.kind"`.

- [ ] T014 [P] [US3] In `internal/mcpserver/server.go`: update `mcp.WithString("edits", mcp.Description(...))` for `submit_tailor_t1` to the new description per `contracts/submit_tailor_t1.md §Tool Registration` — include `category` field, flat example, and categorized example
- [ ] T015 [P] [US3] In `internal/mcpserver/prompt.go`: update `workflowPromptText` — (1) change `submit_tailor_t1` tool table row to show `category?` in edit schema; (2) update Step 5 T1 instruction to distinguish flat vs categorized edit construction per `contracts/submit_tailor_t1.md §Workflow Prompt`
- [ ] T016 [US3] Run `go test ./... -race` and confirm no regressions; confirm updated strings are present in `server.go` and `prompt.go`

**Checkpoint**: Agent B work complete — open PR targeting `dev`. All tests green.

---

## Phase 4: Polish & Merge

**Purpose**: Ordered merge to maintain clean history. Agent A's port change must land in `dev` before Agent B's PR is merged (Agent B's MCP strings are semantically independent but the `Category` field should be in `dev` first for a coherent merge sequence).

- [ ] T017 Review and merge Agent A PR (port + service layer) into `dev` — `go test ./...` must pass on `dev` post-merge
- [ ] T018 Review and merge Agent B PR (MCP adapter) into `dev` after Agent A — `go test ./...` must pass on `dev` post-merge
- [ ] T019 [P] Delete Agent A worktree, local branch, and remote branch after merge
- [ ] T020 [P] Delete Agent B worktree, local branch, and remote branch after merge

---

## Dependencies & Execution Order

### Phase Dependencies

- **Foundational (Phase 1)**: No dependencies — start immediately
- **Service Layer (Phase 2)**: Depends on T001 (Category field) — Agent A
- **MCP Adapter (Phase 3)**: Can start after T001 — Agent B — **runs in parallel with Phase 2**
- **Polish (Phase 4)**: Depends on Phases 2 and 3 both complete

### User Story Dependencies

- **US1 + US2**: Both served by Agent A — same files, same `applySkillsEdit` function. Implement together.
- **US3**: Agent B — independent files. No dependency on US1/US2 being complete.

### Within Each Phase

- Tests (T002–T008) should be written and verified FAILING before implementation (T010–T012)
- Helpers T010 and T011 can be written in parallel before routing logic T012
- Agent B tasks T014 and T015 are fully independent of each other

---

## Parallel Execution: Dispatch After T001

```
# After T001 completes, dispatch both agents simultaneously:

Agent A (Sonnet) — Service Layer
  worktree: feature/005-agent-a-service
  sequence: T002 → T003–T008 [parallel] → T009 → T010,T011 [parallel] → T012 → T013

Agent B (Haiku) — MCP Adapter
  worktree: feature/005-agent-b-mcp
  sequence: T014, T015 [parallel] → T016
```

---

## Implementation Strategy

### MVP (US1 + US2 — Agent A Only)

1. Complete Phase 1: T001
2. Complete Phase 2: T002–T013
3. **STOP and VALIDATE**: `go test ./internal/service/tailor/... -race -v`
4. Open Agent A PR — US1 and US2 are fully functional

### Full Delivery (All Three User Stories)

1. T001 (port field) — unblocks both agents
2. Dispatch Agent A and Agent B in parallel
3. Agent A PR merges first (port change lands in dev)
4. Agent B PR merges second
5. All three user stories complete

---

## Notes

- All tasks are modifications to existing files — no new files created
- Agent A: `internal/port/tailor.go` + `internal/service/tailor/` (Sonnet — non-trivial logic)
- Agent B: `internal/mcpserver/` (Haiku — mechanical string updates)
- Merge order: A first, B second — per `plan.md §Parallelization Strategy`
- `go test -race ./...` required before any PR; pre-commit hook enforces ≥80% coverage
- No schema version bump, no sidecar format change, no new interfaces
