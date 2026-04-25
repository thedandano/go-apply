# Feature Specification: Pretty Log Output

**Feature Branch**: `006-pretty-log-output`  
**Created**: 2026-04-25  
**Status**: Draft  
**Input**: User description: "i want to make log outputs pretty. specifically the json blobs they are stringafied and i would like them to be structured and easy to read something like pretty print in python"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Readable JSON in Logs via `go-apply logs` (Priority: P1)

A developer runs `go-apply logs` (or `go-apply logs --follow`) to view application log output. Log lines follow a logfmt-style format: `YYYY-MM-DD HH:MM:SS LEVEL message key=value key=value ...`. Some field values are quoted JSON strings (e.g., `result="{\"previous_score\":75.875,...}"`). Currently those appear as a single compact escaped string on the same line, making it very hard to read. After this change, the `go-apply logs` command detects any field value that is a valid JSON string and displays it pretty-printed — with indentation and line breaks like Python's `json.dumps(obj, indent=2)` — so the developer can understand the data structure immediately without additional tooling. The underlying log lines remain unchanged.

**Why this priority**: This is the core request. Readable display output reduces debugging time and is the primary value delivered.

**Independent Test**: Can be fully tested by running `go-apply logs` against a log source containing a JSON payload and verifying the displayed output is indented and human-readable.

**Acceptance Scenarios**:

1. **Given** a log entry containing a JSON object stored as a compact string (e.g., `result="{\"previous_score\":75.875,...}"`), **When** the developer runs `go-apply logs`, **Then** the non-JSON fields appear on the first line and the JSON field is displayed below it, indented under a `key:` label with 2-space indented JSON — for example:
   ```
   2026-04-25 10:30:58 DEBU mcp tool result tool=submit_tailor_t2 status=ok result_bytes=1697
     result:
       {
         "previous_score": 75.875,
         "new_score": { ... }
       }
   ```
2. **Given** a log entry containing a deeply nested JSON object, **When** the developer runs `go-apply logs`, **Then** nested objects and arrays in the display are each indented one additional level relative to their parent.
3. **Given** a log entry that contains no JSON fields, **When** the developer runs `go-apply logs`, **Then** the line is displayed as a single unchanged line.
4. **Given** a log entry containing multiple JSON fields, **When** the developer runs `go-apply logs`, **Then** each JSON field is printed below the header line under its own `key:` label, in order.
5. **Given** the developer runs `go-apply logs --follow`, **When** new log entries arrive, **Then** each entry is pretty-printed in real time using the same formatting rules.

---

### User Story 2 - Underlying Logs Remain Compact and Parseable (Priority: P2)

A developer or automated system reads the raw log file or stream directly (not via `go-apply logs`). After this change, the underlying log format is completely unaffected — logs remain compact and machine-readable, safe for ingestion by log aggregators, CI pipelines, or alerting systems.

**Why this priority**: Pretty-printing must not corrupt the stored log format. The display layer and the storage layer must be independent.

**Independent Test**: Can be tested by reading the raw log output directly (bypassing `go-apply logs`) and verifying it is byte-for-byte identical to pre-feature output.

**Acceptance Scenarios**:

1. **Given** a log entry with a JSON payload, **When** read directly from the log source (not via `go-apply logs`), **Then** the entry is identical to how it would appear before this feature.
2. **Given** a log entry where the JSON payload is malformed (not valid JSON), **When** displayed via `go-apply logs`, **Then** the raw string is passed through unchanged and no crash occurs.

---

### User Story 3 - Consistent Pretty-Print Across All Log Callsites (Priority: P3)

A developer reviews logs from multiple parts of the application and finds the JSON formatting consistent everywhere. They do not need to opt in per callsite — any place that currently logs a JSON string benefits automatically.

**Why this priority**: Consistency reduces cognitive overhead when reading multi-component logs.

**Independent Test**: Can be tested by triggering log output from at least two different callsites and verifying identical formatting style.

**Acceptance Scenarios**:

1. **Given** multiple distinct callsites that each log a JSON payload, **When** all entries are written, **Then** the formatting style (indentation character, nesting depth representation) is identical across all entries.

---

### Edge Cases

- What happens when a log field contains a JSON string that is itself escaped (e.g., `"{\"key\":\"value\"}"`)? The system should detect and pretty-print the inner JSON.
- What happens when the JSON blob is very large (thousands of fields)? Output should still be formatted without truncation, but this may be revisited if it causes performance problems.
- What happens when a log field contains a value that looks like JSON but is invalid? The raw value is passed through unchanged.
- What happens when `go-apply logs` output is piped to another command? Pretty-printing still applies since it is a display-layer concern; the underlying logs remain compact regardless.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The `go-apply logs` display renderer MUST detect JSON-encoded string values (objects and arrays only — primitives such as strings, numbers, booleans, and null stay on the header line) within each logfmt field and display them in a split layout: non-JSON fields on the header line, each JSON field printed below under a `key:` label (see FR-004 for indentation).
- **FR-002**: The renderer MUST preserve all non-JSON fields in a log entry exactly as stored — only the JSON value fields are reformatted in the display.
- **FR-003**: The renderer MUST handle invalid JSON gracefully by displaying the raw value unchanged, without crashing or omitting the field.
- **FR-004**: The renderer MUST apply consistent indentation: the `key:` label at 2-space offset from the line start; JSON body at 4-space total depth from the line start (2-space `MarshalIndent` increment relative to the label).
- **FR-005**: The renderer MUST handle nested JSON objects and arrays, indenting each level relative to its parent in the display.
- **FR-006**: The renderer MUST apply to all log levels (debug, info, warn, error) visible via `go-apply logs`, without requiring opt-in per entry.
- **FR-007**: The underlying log storage format MUST NOT be modified by this feature — compact log format is preserved for all non-display consumers.
- **FR-008**: The `--follow` flag MUST apply the same pretty-print rendering in real time as new entries are streamed.

### Key Entities

- **Log Entry**: A single line of log output in logfmt style: `YYYY-MM-DD HH:MM:SS LEVEL message key=value key=value ...`. Some field values are quoted strings that contain JSON.
- **JSON Payload**: A string-encoded JSON object or array found within a log entry field that should be pretty-printed.
- **Log Display Renderer**: The component within `go-apply logs` responsible for transforming stored log entries into a human-readable format for display. Distinct from the write path.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Developers can read a logged JSON object and identify its top-level keys within 5 seconds without additional tooling.
- **SC-002**: 100% of log entries containing valid JSON are displayed with consistent indentation when viewed via `go-apply logs`.
- **SC-003**: 0% of non-JSON metadata fields are lost or altered in the display output.
- **SC-004**: Raw log sources are byte-for-byte identical to pre-feature output — only the display layer changes.

## Assumptions

- Pretty-printing is a display-layer concern applied by `go-apply logs` at read time; the underlying log write path is not modified.
- Log lines use logfmt-style formatting: `YYYY-MM-DD HH:MM:SS LEVEL message key=value ...`. Field values that are quoted strings may contain JSON.
- The JSON detection strategy: for each `key=value` pair where the value is a quoted string, attempt to parse the unquoted string as JSON. If it parses as a JSON object or array, pretty-print it in the display.
- Log consumers other than `go-apply logs` (dashboards, alerting, CI pipelines) are unaffected because raw log format is unchanged.
- `go-apply logs --follow` tails the log source in real time; pretty-printing applies to each entry as it arrives.
- Toggling pretty-print off (e.g., via flag or env var) is out of scope for this version.

## Clarifications

### Session 2026-04-25

- Q: Should pretty-printing apply to all output destinations or only when viewed via `go-apply logs`? → A: Only in the `go-apply logs` (and `go-apply logs --follow`) display layer; underlying log format stays compact.
- Q: What is the log entry format? → A: Logfmt-style plain text — `YYYY-MM-DD HH:MM:SS LEVEL message key=value ...` — some field values are quoted JSON strings (e.g., `result="{\"previous_score\":...}"`)
- Q: How should pretty-printed JSON appear relative to the rest of the log line? → A: Split layout — non-JSON fields stay on the header line; each JSON field is printed below it, indented under a `key:` label with 2-space indented JSON.
