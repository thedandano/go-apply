# Feature Specification: Preserve Finalize Output and Pretty-Print Logs

**Feature Branch**: `002-preserve-finalize-logs`
**Created**: 2026-04-23
**Status**: Draft
**Input**: User description: "finalize should not clear all the output when a run finishes and logs should have some sort of pretty print"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Finalize Preserves Run Output (Priority: P1)

A developer using the MCP server calls `finalize` at the end of a session. After finalize completes, all log output and stage banners from the current run remain visible in the terminal/Claude Code output pane rather than being replaced or cleared.

**Why this priority**: The run log is the primary observability surface. If it disappears when finalize is called, the user loses all context about what happened during the session — scores, tailoring steps, errors — at the exact moment they need to review results.

**Independent Test**: Can be fully tested by running a complete MCP session through `load_jd → submit_keywords → finalize` and confirming that all slog output emitted before `finalize` is still visible after `finalize` returns.

**Acceptance Scenarios**:

1. **Given** an MCP session has completed `load_jd` and `submit_keywords`, producing multiple log lines, **When** `finalize` is called, **Then** all previously emitted log lines remain present in the output and are not removed or overwritten.
2. **Given** a CLI `run` command that finishes successfully, **When** the pipeline exits, **Then** all stage banners and log messages from the run remain on screen.
3. **Given** a CLI `run` command that exits with an error, **When** the pipeline exits, **Then** all log output up to the error point remains visible.

---

### User Story 2 - Human-Readable Log Formatting (Priority: P2)

A developer reviewing a go-apply run in a terminal sees log output formatted with visual structure: colored level indicators, aligned key-value pairs, and readable timestamps — rather than raw text strings or unformatted key=value output.

**Why this priority**: The project already depends on `charmbracelet/log` for the stderr handler, so the infrastructure exists. The gap is that the formatting may not be fully applied or may be inconsistently styled across the run lifecycle.

**Independent Test**: Can be fully tested by running `go-apply run` with a valid JD URL and confirming that the stderr output contains colored/styled log lines with visible level prefixes and structured key-value alignment.

**Acceptance Scenarios**:

1. **Given** the tool runs in a terminal with color support, **When** any log message is emitted at any level (DEBUG, INFO, WARN, ERROR), **Then** the level is visually distinguished (e.g., colored prefix or badge), and key-value pairs are aligned and readable.
2. **Given** the tool runs in a non-terminal environment (piped/redirected output), **When** log messages are emitted, **Then** ANSI color codes are stripped and plain text is emitted.
3. **Given** the `LOG_FORMAT=json` environment variable is set, **When** log messages are emitted, **Then** output is machine-readable JSON regardless of terminal detection.

---

### User Story 3 - Finalize Summary Pretty-Printed (Priority: P3)

When `finalize` completes (either via MCP tool or CLI), the run summary is presented in a human-readable format that makes the key results — best resume, score, cover letter status — immediately scannable.

**Why this priority**: The run result is currently a raw JSON blob. A formatted summary at the end of the run helps the user immediately understand the outcome without parsing JSON.

**Independent Test**: Can be fully tested by completing a run and confirming that a formatted summary block appears in the output alongside (not replacing) the existing JSON result.

**Acceptance Scenarios**:

1. **Given** `finalize` is called with a valid scored session, **When** the tool returns, **Then** a human-readable summary block is emitted to stderr/log showing best resume label, best score, and cover letter length.
2. **Given** a CLI `run` completes, **When** `ShowResult` is called, **Then** the formatted summary appears in the output before or after the JSON blob.

---

### Edge Cases

- What happens when the terminal does not support colors (e.g., `TERM=dumb`)? The formatter must fall back to plain text without ANSI codes.
- What happens when `LOG_FORMAT=json` is set but the user also wants human-readable output? The `LOG_FORMAT` override takes precedence; no pretty-printing in JSON mode.
- What happens if `finalize` is called multiple times for the same session? The clearing behavior should not occur on any invocation; the second call should return an error without clearing existing output.
- What happens when the run produces zero log lines before finalize? Finalize should still not clear output; it should simply emit its own summary without affecting an empty output pane.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The `finalize` MCP tool MUST NOT emit any terminal escape sequences that clear, overwrite, or scroll past previously displayed output.
- **FR-002**: The CLI `run` command MUST NOT clear the terminal on exit or error.
- **FR-003**: Log messages emitted to a terminal MUST be formatted using the charmbracelet/log pretty handler with colored level indicators and aligned key-value pairs.
- **FR-004**: Log messages emitted to a non-TTY output (file, pipe) MUST omit ANSI escape codes; file handler MUST remain JSON format.
- **FR-005**: Terminal detection for pretty-print vs. plain-text MUST use TTY detection (isatty) and MUST be overridable via the `LOG_FORMAT=json` environment variable.
- **FR-006**: When `finalize` completes, a human-readable summary of the run MUST be emitted to the log output (stderr) showing at minimum: best resume label, best score, and cover letter character count.
- **FR-007**: The pretty-print formatting MUST be consistent across all pipeline stages (Session banner, Score banner, Tailor steps, Finalize summary).

### Key Entities

- **LogHandler**: The slog handler responsible for routing log records to file and stderr; must apply per-output formatting.
- **FinalizeResult**: The structured summary emitted after a finalize call, rendered both as JSON (MCP response) and as a formatted log block (stderr).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero ANSI clear-screen sequences (`\x1b[2J`, `\x1b[H`) appear in the combined stdout+stderr output of any run that reaches `finalize`.
- **SC-002**: 100% of log lines emitted to a TTY-connected stderr include a visible level indicator (colored or prefixed) when `LOG_FORMAT` is not set to `json`.
- **SC-003**: All key-value pairs in a log line align within the same column boundary as adjacent log lines in the same run.
- **SC-004**: A user can identify the best resume and score from the finalize summary without parsing the JSON result blob.
- **SC-005**: When `LOG_FORMAT=json` is set, zero charmbracelet styling or ANSI sequences appear in any output.

## Assumptions

- The perceived "clearing" effect is caused by the run completing silently with no log output after the final stage — not by an explicit clear-screen escape sequence. Adding a summary log block resolves this.
- The finalize summary is emitted via `slog.InfoContext` which fans to both the log file and stderr via the existing `multiHandler`. Appearing on both sinks is acceptable.
- `charmbracelet/log` already provides human-readable formatting for file output (auto-strips ANSI on non-TTY writers) — no new dependency or logger change required.
- The JSON stdout contract (headless mode) is unchanged; the summary never goes to stdout.
- Mobile and web rendering of logs is out of scope; the feature targets terminal and log-file output only.
