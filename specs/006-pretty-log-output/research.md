# Research: Pretty Log Output

**Date**: 2026-04-25  
**Status**: Complete — all decisions resolved from codebase

## D-001: Renderer location

- **Decision**: `internal/cli/logs_render.go` (same `cli` package as the `logs` command)
- **Rationale**: Display-only concern; no reason to cross a package boundary for a single function
- **Alternatives considered**: `internal/logger/` — rejected, that package owns the write path

## D-002: JSON detection strategy

- **Decision**: Regex `(\w+)=("[^"\\]*(?:\\.[^"\\]*)*")` → `strconv.Unquote` → `json.Unmarshal`; only `{` and `[` first bytes trigger pretty-print
- **Rationale**: Charmbracelet/log uses Go's `strconv.Quote` for string values; `strconv.Unquote` is the correct inverse. Limiting to objects/arrays avoids false positives on plain strings like `status="ok"`.
- **Alternatives considered**: Regex-only JSON detection — rejected (cannot reliably detect balanced braces)

## D-003: No new dependencies

- **Decision**: stdlib only (`encoding/json`, `strconv`, `regexp`, `strings`, `io`)
- **Rationale**: All required functionality is available in the standard library. Adding a logfmt parsing library (e.g., `kr/logfmt`) would be overkill for the narrow transform needed here.

## D-004: Sensitive data

- **Decision**: No additional redaction in the renderer
- **Rationale**: `payload.go` already applies `Redact()` before values reach the log file. The renderer operates on already-redacted stored content.
