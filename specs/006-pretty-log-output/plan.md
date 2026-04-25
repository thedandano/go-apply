# Implementation Plan: Pretty Log Output

**Branch**: `006-pretty-log-output` | **Date**: 2026-04-25 | **Spec**: [spec.md](spec.md)  
**Input**: Feature specification from `specs/006-pretty-log-output/spec.md`

## Summary

`go-apply logs` currently prints raw logfmt lines byte-for-byte from the log file. Some field values are quoted JSON strings (e.g., `result="{\"previous_score\":75.875,...}"`). This feature adds a display renderer that detects those fields and prints them in a split layout: non-JSON fields on a single header line, each JSON field indented below under a `key:` label. The underlying log file format is not modified.

## Technical Context

**Language/Version**: Go 1.26  
**Primary Dependencies**: stdlib only (`encoding/json`, `strconv`, `regexp`, `strings`) — no new third-party deps  
**Storage**: N/A — read-only display transform  
**Testing**: `go test ./...`, race detector via `go test -race ./...`  
**Target Platform**: All platforms (CLI tool)  
**Project Type**: CLI  
**Performance Goals**: Imperceptible overhead for interactive log viewing  
**Constraints**: No new package-level imports; no changes to log write path or file format  
**Scale/Scope**: Per-line transform of charmbracelet/log logfmt output

## Constitution Check

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Vertical Slicing | ✅ Pass | Single user story: readable JSON in `go-apply logs`. End-to-end value from command invocation to display. |
| II. Test-First | ✅ Pass | Unit tests for `renderLine` written before implementation; existing `logs_test.go` extended |
| III. Hexagonal Architecture | ✅ Pass | Change is in `internal/cli/` (presenter layer). No import violations. No new packages importing presenters. |
| IV. No Silent Failures | ✅ Pass | Invalid JSON passes through unchanged (no panic, no swallowed error). No new error paths added. |
| V. Observability | ✅ Pass | Log write path untouched. Display renderer is pure transform with no side effects. |

No violations. No complexity tracking required.

## Project Structure

### Documentation (this feature)

```text
specs/006-pretty-log-output/
├── plan.md              ← this file
├── research.md          ← Phase 0 output (inline below — no external research needed)
├── data-model.md        ← Phase 1 output (inline below — no persistent entities)
├── contracts/           ← Phase 1 output
└── tasks.md             ← Phase 2 output (/speckit-tasks command)
```

### Source Code (affected files)

```text
internal/cli/
├── logs.go              ← modify: wire renderLine into runLogs
├── logs_render.go       ← new: renderLine function
├── logs_render_test.go  ← new: unit tests for renderLine
└── logs_test.go         ← extend: add integration test for JSON rendering
```

## Phase 0: Research

No external research required. All decisions resolve from the existing codebase.

### Decision Log

**D-001: Where does the renderer live?**  
Decision: `internal/cli/logs_render.go` — a new file within the existing `cli` package.  
Rationale: The renderer is a display concern tightly coupled to the `logs` command. Adding it to the same package avoids a new package boundary for a single function.  
Alternatives considered: `internal/logger/render.go` — rejected because the `logger` package owns the write path; mixing display logic there violates separation of concerns.

**D-002: How to detect JSON field values?**  
Decision: Scan for `key="value"` pairs using a compiled regex. For each quoted value, use `strconv.Unquote` to decode Go string escaping, then `json.Unmarshal` to check validity. Only object (`{}`) and array (`[]`) results trigger pretty-printing.  
Rationale: Charmbracelet/log quotes string values using Go's `strconv.Quote` format. `strconv.Unquote` is the correct inverse. Limiting detection to objects and arrays avoids false positives on plain quoted strings like `status="ok"`.  
Alternatives considered: Regex-only JSON detection — rejected because regex cannot reliably detect balanced braces.

**D-003: What stays on the header line?**  
Decision: All key=value pairs that are NOT pretty-printed JSON remain on the header line, preserving their original text verbatim (including the prefix: timestamp, level, message).  
Rationale: Reconstructing the line from parsed tokens risks subtle formatting differences. Preserving the original text of non-JSON pairs exactly matches the underlying log file.  
Alternatives considered: Full logfmt parse and reconstruct — rejected as unnecessary complexity with no user-visible benefit.

**D-004: Indentation style**  
Decision: JSON fields are printed with `  key:\n` (2-space indent for the label) followed by the JSON body indented with `    ` (4 spaces = 2 for label + 2 for JSON content). `json.MarshalIndent` uses `"  "` (2-space) for JSON indentation.  
Rationale: Matches the example approved during clarification. 4-space total for JSON content makes it visually distinct from the header.

**D-005: Log line regex pattern**  
Decision: Use `(\w+)=("[^"\\]*(?:\\.[^"\\]*)*")` to find all `key="value"` pairs. This handles Go-escaped strings (e.g., `\"`). Non-quoted values (`key=value`) are not candidates for JSON detection.  
Rationale: Charmbracelet/log only quotes strings that contain special characters; bare strings (e.g., `session_id=abc123`) are not quoted. Only quoted values can contain embedded JSON.

## Phase 1: Design & Contracts

### Data Model

No persistent entities. This feature is a pure display-layer transform.

**Input**: A raw log line string (UTF-8) read from a charmbracelet/log logfmt file.  
**Output**: One or more display lines written to `io.Writer`:
- Line 1: The original log line with JSON-valued fields removed (header)
- Line 2..N: For each JSON field (in original order): `  key:\n    <indented JSON>`

**Field**: `key="value"` where `value` is a Go-quoted string whose unquoted content is a valid JSON object or array.

### Renderer Contract

```go
// renderLine writes a pretty-printed representation of a logfmt log line to w.
// Fields whose quoted string values are valid JSON objects or arrays are removed
// from the header line and printed below it, indented, under a "key:" label.
// Lines containing no JSON-valued fields are written unchanged.
// Errors from w are returned.
func renderLine(w io.Writer, line string) error
```

**Invariants**:
- A line with no JSON-valued fields produces exactly one output line, identical to the input.
- A line with N JSON-valued fields produces 1 + N×(1 + body_lines) output lines.
- `renderLine` never panics regardless of input.
- Invalid JSON in a quoted field value → field stays in header line, no pretty-print.

### CLI Contract (no change to interface)

`go-apply logs [--follow] [-n N] [--log-dir DIR]` — flags and behavior unchanged. Output format changes only for lines containing JSON-valued fields.

### Agent Context Update

CLAUDE.md updated to point to this plan (see Step 3 in Phase 1 instructions).

## Implementation Notes (for task generation)

### Task 1 — Write failing unit tests for `renderLine` (TDD Red)

File: `internal/cli/logs_render_test.go`  
Package: `cli_test`

Test cases:
1. Line with no quoted fields → output identical to input
2. Line with one JSON object field → header without that field, JSON block below
3. Line with one JSON array field → same as above
4. Line with multiple JSON fields → header cleaned, each JSON block in order
5. Line with a quoted non-JSON string field → field stays on header line
6. Line with invalid (malformed) JSON in a quoted value → field stays on header, no panic
7. Line with deeply nested JSON → correct indentation at all levels
8. Empty line → empty output (no panic)
9. Real-world line from the example in the spec: `2026-04-25 10:30:58 DEBU mcp tool result tool=submit_tailor_t2 session_id=... status=ok result_bytes=1697 result="{\"previous_score\":75.875,...}"`

### Task 2 — Implement `renderLine` (TDD Green)

File: `internal/cli/logs_render.go`  
Package: `cli`

Implementation outline:
```
1. Compile regex (package-level var): (\w+)=("[^"\\]*(?:\\.[^"\\]*)*")
2. Find all matches in line
3. For each match: strconv.Unquote the value, json.Unmarshal into json.RawMessage
4. If valid JSON and first byte is '{' or '[': mark as "JSON field"
5. Build header: line with each JSON field's full match text removed (preserve surrounding whitespace)
6. Trim trailing whitespace from header
7. Write header + "\n"
8. For each JSON field in order: json.MarshalIndent → write "  key:\n    <indented body>\n"
   - Indent prefix for MarshalIndent: "    " (4 spaces)
9. Return first write error encountered
```

### Task 3 — Wire `renderLine` into `runLogs`

File: `internal/cli/logs.go`  
Change: In `runLogs`, replace:
```go
fmt.Fprintln(cmd.OutOrStdout(), line)
```
with:
```go
if err := renderLine(cmd.OutOrStdout(), line); err != nil {
    return fmt.Errorf("render line: %w", err)
}
```
Apply in both the initial tail loop and the `--follow` ticker loop.

### Task 4 — Add integration test in `logs_test.go`

Extend `internal/cli/logs_test.go`:
- Write a fixture log file containing a real charmbracelet/log-style line with a JSON payload field
- Run `go-apply logs` and assert:
  - Header line contains timestamp, level, message, and non-JSON fields
  - Header line does NOT contain the JSON field value
  - Output contains `  result:` label
  - Output contains `    "previous_score"` (indented JSON key)
  - Non-JSON lines in the same file are printed unchanged

### Sensitive Data Note

`payload.go` already applies `Redact()` before values reach the log file. The renderer operates on already-redacted stored values — no additional redaction needed in the display layer.
