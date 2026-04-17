# Retro: Unified Retrieval-at-Tailoring-Time + Verbose Diffs + PII Redaction

**Date:** 2026-04-17
**PRs:** #86 (feat/retrieval-at-tailoring-time → dev)

## What Was Built

Four coupled problems addressed in a single squash-merged PR:

- **Part A — Retrieval moved to tailoring time:** Removed `AugmentResumeText` from `scoreResumes`; scoring now always uses raw resume text. Added `SuggestForKeywords(ctx, []string) (TailorSuggestions, error)` to `port.Augmenter` and implemented it in `augment.Service` (vector with keyword fallback, no LLM). Embedded the retrieval call inside `runTailorStep` for both T1 and T2. Added invariant comment at `scoreResumes`.
- **Part A.4 — `suggest_tailoring` MCP tool:** Optional diagnostic tool exposing what the profile bank found for missing JD keywords. Session-state-gated (requires `stateScored`). Returns `{required, preferred, retrieval_mode}`. Not a required step — T1/T2 retrieve internally in headless mode; in MCP mode the orchestrator provides inputs directly.
- **Part B — `internal/debugdump` package:** `Dump` (pp/v3, colors off), `DiffText` (hexops/gotextdiff unified diff), `DiffSection` (section-scoped diff). T1 Skills diff and per-T2-bullet diff emitted behind `logger.Verbose()`.
- **Part C — `internal/redact` package:** Word-boundary name masking (`\b` + `regexp.QuoteMeta`), RFC-5322 email, NANP + E.164 phone patterns. `atomic.Pointer[redact.Redactor]` in `logger.PayloadAttr`. Installed at startup in MCP and headless modes. `debug.disable_redaction` config opt-out.
- **Post-review fixes (same PR):** Surfaced three silent fallback categories as `RiskWarning` entries in `PipelineResult.Warnings`: per-resume load/score failures, `SuggestForKeywords` failure in `runTailorStep`, and T2 all-LLM-fail. `rewriteBullets` returns an `attempted` count; `TailorResult` carries `BulletsAttempted`; `tailor.go` uses a switch to emit distinct Decision reasons.

## What Went Well

- The state-machine framing (single pipeline, two drivers) was the right mental model and kept the design coherent across MCP and headless modes without duplicating logic.
- `atomic.Pointer` for the global redactor was the correct synchronization primitive — caught during review before shipping.
- The `RiskWarning` pattern (already established in `PipelineResult`) made surfacing the new warnings straightforward: no new types, no new contracts.
- Opus's review was thorough and found real issues: the T2 all-fail silent degradation was the most significant one, and the suggested fix (count field on `TailorResult`) was minimal and correct.

## What Was Harder Than Expected

- **Silent fallback proliferation:** One obvious silent fallback (AccomplishmentsText synthesis from Suggestions) was caught and removed during the session. Opus's review then found three more structural cases (resume skip, SuggestForKeywords, T2 all-fail) that required a second commit. Large PRs need explicit silent-failure audits, not just code review.
- **`RedactAny` unexported field panic:** `reflect.Value.Set` on unexported struct fields panics. The fix (skip unexported fields, leave them zero-valued) was correct but the initial implementation missed it. Adding a test with a struct containing unexported fields would have caught this before the pre-push hook.
- **`suggest_tailoring` semantic drift:** The plan said T1/T2 retrieve internally in all modes. Reality: in MCP mode the orchestrator provides `skill_adds`/`bullet_rewrites` directly — there is no internal retrieval. The `prompt.go` initially contained a false claim. Required a correction commit and a clearer mental model of "headless = internal retrieval, MCP = orchestrator-provided inputs."
- **Real name in test:** `TestRedact_NameCaseInsensitive` was written using the actual user's name. Caught by the user. Replaced with "Jane Smith" and amended.

## What to Do Differently

- **Silent-failure audit as a checklist item:** Before opening a PR of this size, explicitly grep for `continue` inside loops and `slog.Warn` calls not paired with `result.Warnings` appends. The pattern "log and continue" without surfacing to the structured result is the footprint of this class of bug.
- **Test unexported-field structs in `RedactAny`:** Any reflect-based walker needs a test case with unexported fields to catch the panic path before shipping.
- **Clarify driver model in the plan:** The distinction between "headless = pipeline retrieves internally" and "MCP = orchestrator provides inputs" should be an explicit callout in the plan, not something resolved during implementation. It caused two corrections (prompt.go claim, suggest_tailoring semantics).
- **Don't use real names in tests:** Even in private repos, fixture data should use obviously synthetic identifiers. Catch with a pre-commit grep for `git config user.name` value in test files.
