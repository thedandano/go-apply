# Implementation Plan: Preserve Finalize Output and Pretty-Print Logs

**Branch**: `002-preserve-finalize-logs` | **Date**: 2026-04-23 | **Spec**: [spec.md](./spec.md)

## Summary

Two related gaps in the finalize flow:

1. **No run summary logged** — `HandleFinalizeWithConfig` (and `pipeline.Run`) complete silently; the only artifacts are the JSON tool result (MCP) or JSON blob on stdout (CLI). Nothing is written to the log file or stderr that summarises what happened.
2. **Pretty-print already exists** — `charmbracelet/log` is already wired as the stderr/file handler via `multiHandler`. When writing to a non-TTY (log file), it auto-strips ANSI codes and emits clean plain-text. No new dependency needed.

**Approach**: Add a structured `slog.InfoContext` summary block inside `HandleFinalizeWithConfig` and at the end of `pipeline.Run`, using the existing `logger.Banner` helper for visual separation. Both paths already call `slog.Default()`, so the summary will fan-out to the log file and stderr via the existing `multiHandler` for free.

## Technical Context

**Language/Version**: Go 1.23+
**Primary Dependencies**: `charmbracelet/log v1.0.0` (already present), `log/slog` (stdlib)
**Storage**: Daily log file at `~/.local/share/go-apply/logs/go-apply-YYYY-MM-DD.log`
**Testing**: `go test ./...`, `go test -race ./...`
**Target Platform**: macOS/Linux terminal + Claude Code MCP subprocess
**Project Type**: CLI + MCP server
**Performance Goals**: No latency impact — log writes are synchronous but negligible for a summary block
**Constraints**: stdout must remain JSON-only (headless contract); summary goes to slog only
**Scale/Scope**: Two call sites — `HandleFinalizeWithConfig` (MCP) and `pipeline.Run` (CLI)

## Constitution Check

| Gate | Status | Notes |
|------|--------|-------|
| Tests before implementation (TDD) | REQUIRED | Write `slog` capture tests first |
| No silent failures | PASS | Adding log output, not removing error paths |
| Structured logging | PASS | Using `slog.InfoContext` with named fields |
| Stdout = JSON only | PASS | Summary goes to `slog`, not `fmt.Println` to stdout |
| Architecture invariant (no presenter→service import) | PASS | Changes are in service/mcpserver layers |

## Project Structure

### Documentation (this feature)

```text
specs/002-preserve-finalize-logs/
├── plan.md              ← this file
├── research.md          ← Phase 0 output
├── data-model.md        ← N/A (no new data model)
└── tasks.md             ← Phase 2 output (/speckit-tasks)
```

### Source Code (affected files)

```text
internal/
├── mcpserver/
│   └── session_tools.go     # add summary slog block in HandleFinalizeWithConfig
├── service/pipeline/
│   └── apply.go             # add summary slog block at end of Run()
└── logger/
    └── banner.go            # extend with FinalizeResult logger (or inline in callers)
```

## Phase 0: Research

### R-001: What causes the "clearing" effect?

**Finding**: No explicit clear-screen sequences (`\x1b[2J`) exist anywhere in the codebase. The `charmbracelet/log` handler does not use alternate screen buffers or progress spinners. The perceived "clearing" is most likely that nothing is logged after the final pipeline stage — the run ends silently (only a JSON blob on stdout), so the terminal appears to have "lost" the context of what happened.

**Decision**: Fix by adding an explicit run-summary log block at the point of finalization.
**Alternatives considered**: Alt-screen TUI (out of scope), separate `fmt.Println` to stdout (breaks headless contract).

### R-002: Log file format with charmbracelet/log on a non-TTY

**Finding**: `charmbracelet/log.NewWithOptions` auto-detects `isatty` on the writer. When the writer is a file (not a TTY), it strips ANSI codes and emits plain text with aligned columns: `TIMESTAMP LEVEL message key=value`. This is already "pretty print" for the log file.

**Decision**: No changes needed to the logger itself. The existing setup already produces clean, readable log files.

### R-003: Suppressing stderr output (user asked)

**Finding**: `logger.New` sets `FileLevel` and `StderrLevel` to the same value from `cfg.ResolveLogLevel()`. Both sinks receive the same events. To suppress summary from stderr would require a file-only logger or a level trick. User confirmed: showing on stderr too is acceptable.

**Decision**: Use `slog.InfoContext` as-is; the summary appears in both the log file and on stderr.

## Phase 1: Design

### What to log at finalize

Both call sites (MCP `HandleFinalizeWithConfig` and CLI `pipeline.Run`) should emit the same structured summary at `slog.Info` level using a `logger.Banner` header followed by key fields:

```
***************************
Finalize
***************************
best_resume=<label>  best_score=<float>  cover_letter_chars=<int>  resumes_scored=<int>
```

This uses the existing `logger.Banner(ctx, slog.Default(), "Finalize", "")` helper plus one `slog.InfoContext` call with named attributes.

### Call site 1 — MCP: `HandleFinalizeWithConfig` (`internal/mcpserver/session_tools.go`)

Insert after `sess.State = stateFinalized` and before the `return`:

```go
logger.Banner(ctx, slog.Default(), "Finalize", sessionID)
slog.InfoContext(ctx, "run complete",
    slog.String("best_resume", resultData.BestResume),
    slog.Float64("best_score", resultData.BestScore),
    slog.Int("cover_letter_chars", resultData.Summary.CoverLetterChars),
    slog.Int("resumes_scored", resultData.Summary.ResumesScored),
)
```

### Call site 2 — CLI: `pipeline.Run` (`internal/service/pipeline/apply.go`)

Insert after `result.EndTime = time.Now()` and before `return p.presenter.ShowResult(result)`:

```go
logger.Banner(ctx, slog.Default(), "Finalize", "")
slog.InfoContext(ctx, "run complete",
    slog.String("best_resume", result.BestResume),
    slog.Float64("best_score", result.BestScore),
    slog.String("status", result.Status),
)
```

### Test strategy

- Use `logger_test.go` pattern: create a temp-dir logger, call the function under test, assert the log file contains the expected fields.
- For the MCP handler: existing `session_tools_test.go` pattern — capture `slog` output via a test handler and assert the summary keys appear.
- No new test files needed; extend existing test files.

### Contracts

No external interface changes. The MCP tool JSON response is unchanged. The CLI JSON output is unchanged. The log file gains additional `INFO` lines.
