# Tasks: Pretty Log Output

**Input**: Design documents from `specs/006-pretty-log-output/`
**Prerequisites**: spec.md ✅ | plan.md ✅ | research.md ✅ | data-model.md ✅

**TDD**: Tests are written FIRST and must FAIL before implementation begins (constitution requirement).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no shared dependencies)
- **[Story]**: User story this task belongs to
- Include exact file paths in descriptions

---

## Phase 1: Setup

No setup required. The renderer lives in the existing `internal/cli` package. No new dependencies, no new directories.

---

## Phase 2: Foundational

No blocking prerequisites beyond what already exists. All work is within `internal/cli`.

---

## Phase 3: User Story 1 — Readable JSON in `go-apply logs` (Priority: P1) 🎯 MVP

**Goal**: `go-apply logs` displays JSON-valued logfmt fields in a split layout — non-JSON fields on the header line, each JSON field printed below under an indented `key:` label.

**Independent Test**: Run `go-apply logs --log-dir <dir>` against a fixture log file containing a real charmbracelet/log-style line with a JSON field (e.g., `result="{\"score\":75}"`). Verify: header line present without the JSON value; `  result:` label on next line; `    {` on line after.

### Tests for User Story 1 (write FIRST — must FAIL before T002)

- [x] T001 [US1] Write failing unit tests for `renderLine` covering: no JSON fields (line unchanged), single JSON object field (split layout), JSON array field, multiple JSON fields (all moved below), non-JSON quoted string stays on header, malformed JSON stays on header, empty line, the real-world example from the spec, JSON primitive value stays on header (e.g., `result="42"`), and a WARN-level log line with a JSON field renders identically to an INFO-level line — in `internal/cli/logs_render_test.go`

### Implementation for User Story 1

- [x] T002 [US1] Implement `renderLine(w io.Writer, line string) error` in `internal/cli/logs_render.go`: compile regex `(\w+)=("[^"\\]*(?:\\.[^"\\]*)*")`, find all quoted fields, strconv.Unquote each value, json.Unmarshal to detect objects/arrays, rebuild header without JSON fields, write header line then each JSON field as `  key:\n    <json.MarshalIndent output>` — makes T001 pass
- [x] T003 [US1] Wire `renderLine` into `runLogs` in `internal/cli/logs.go`: replace `fmt.Fprintln(cmd.OutOrStdout(), line)` with `renderLine(cmd.OutOrStdout(), line)` in both the initial tail loop and the `--follow` ticker loop; return `fmt.Errorf("render line: %w", err)` on error

**Checkpoint**: At this point, `go-apply logs` should display JSON fields in split layout. All T001 unit tests pass.

---

## Phase 4: User Story 2 — Underlying Logs Remain Compact (Priority: P2)

> **TDD note (constitution §II)**: T004 and T005 must be written and confirmed failing before T003 is considered complete.

**Goal**: Confirm the underlying log file is not modified in any way when `go-apply logs` runs.

**Independent Test**: Capture MD5/byte count of log file before and after running `go-apply logs`; assert identical.

### Tests for User Story 2

- [x] T004 [US2] Add `TestLogsCommand_RawFileUnchangedAfterDisplay` in `internal/cli/logs_test.go`: write fixture log file containing a JSON-valued field, record file size + content hash before running `go-apply logs`, run command, re-read file and assert byte-for-byte identical to pre-run state

**Checkpoint**: T004 confirms the renderer is read-only — no write-path side effects.

---

## Phase 5: User Story 3 — Consistent Formatting Across All Log Entries (Priority: P3)

**Goal**: Verify that all JSON-valued fields in all log entries — regardless of position or field name — are formatted identically.

**Independent Test**: Fixture file with two distinct log entries each containing a JSON field at different positions and with different field names. Both must produce output with identical indentation style and label format.

### Tests for User Story 3

- [x] T005 [US3] Add `TestLogsCommand_ConsistentFormattingAcrossEntries` in `internal/cli/logs_test.go`: write fixture log file with two lines each containing a different JSON-valued field (different field names, different JSON structures); run `go-apply logs`; assert both entries produce `  <key>:\n    {` format with matching indentation depth and style

**Checkpoint**: T005 confirms formatting consistency across all callsites.

- [x] T005a [US3] Add `TestLogsCommand_FollowPrettyPrintsJSON` in `internal/cli/logs_test.go`: start `go-apply logs --follow` against a temp log file, append a JSON-valued log line to the file, read output, and assert the pretty-printed split layout appears in real time (covers FR-008)

---

## Phase 6: Polish & Cross-Cutting

- [x] T006 Run `go test -race ./internal/cli/...` and verify all tests pass; run `go vet ./...`; confirm coverage gate (>= 80%) is met for `internal/cli` package

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 3 (US1)**: No blocking prereqs — can start immediately
- **Phase 4 (US2)**: Depends on T003 (wiring must exist to test read-only behaviour)
- **Phase 5 (US3)**: Depends on T003 (wiring must exist to test consistency)
- **Phase 6 (Polish)**: Depends on T004 and T005

### Within Phase 3 (US1)

```
T001 → T002 → T003
(test)  (impl)  (wire)
```

T001 must exist and FAIL before T002 begins. T002 must pass before T003.

### Within Phases 4 & 5

T004 and T005 both depend on T003. They touch the same file (`logs_test.go`) so run sequentially, but they can be developed in any order after T003.

---

## Parallel Opportunities

### Phase 3 — None

Tasks T001 → T002 → T003 are sequential by TDD discipline.

### Phases 4 & 5 — Limited

T004 and T005 are both in `internal/cli/logs_test.go`. Sequential only (same file). Both can begin as soon as T003 is complete.

---

## Parallel Example: Phase 3

```
# Sequential only (TDD):
Agent: "Write failing unit tests for renderLine in internal/cli/logs_render_test.go"
↓ (tests fail — confirmed)
Agent: "Implement renderLine in internal/cli/logs_render.go to make tests pass"
↓ (tests pass)
Agent: "Wire renderLine into runLogs in internal/cli/logs.go"
```

---

## Implementation Strategy

### MVP (User Story 1 Only)

1. T001 — Write failing tests
2. T002 — Implement `renderLine`
3. T003 — Wire into `runLogs`
4. **STOP and VALIDATE**: `go-apply logs` shows pretty JSON

### Full Delivery

5. T004 — US2 integration test
6. T005 — US3 integration test
7. T006 — Final verification

---

## Notes

- Constitution requires 80% coverage at pre-commit; verify with `go test -coverprofile=coverage.out ./internal/cli/... && go tool cover -func=coverage.out`
- `renderLine` must never panic regardless of input (fuzz-worthy function)
- No new third-party imports — stdlib only (`encoding/json`, `strconv`, `regexp`, `strings`, `io`)
- Commit after T003 (US1 complete) and again after T005 (all stories complete)
