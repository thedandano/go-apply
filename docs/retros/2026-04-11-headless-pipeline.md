# Retro: Task 11 — Headless Pipeline (First Usable Build)

**Date:** 2026-04-11  
**PR:** thedandano/go-apply#13  
**Branch:** feat/task-11-headless

---

## What We Built

Wired all previously-built services into a working `go-apply apply` command:

- `internal/service/pipeline/apply.go` — `ApplyPipeline` with 7-step degraded-mode pipeline
- `internal/presenter/headless/presenter.go` — JSON stdout / events stderr presenter
- `internal/cli/apply.go` — cobra command with all services injected
- `internal/cli/helpers.go` — DB and LLM client constructors

---

## What Went Well

- **Typed-nil avoidance pattern** was caught and documented clearly: `var augmenterSvc port.Augmenter` instead of `var augmenterSvc *augment.Service`. The explanation in the code comment makes it self-documenting.
- **Degraded mode design** is clean — all non-fatal failures follow the same pattern: append `RiskWarning`, set `Status = "degraded"`, continue pipeline.
- **Port/adapter invariant held**: pipeline never imports presenter; architecture check confirmed by spec reviewer.

---

## Bugs Found in Review (not pre-merge)

### Bug 1: ShowError routed to stdout
`ShowError` wrote to `p.out` (stdout) instead of `p.events` (stderr). In headless/agent mode, consumers parse stdout as JSON — an error message on stdout would corrupt the parse. Caught by spec reviewer on first pass.

**Root cause:** Copy-paste from `writeJSON` helper without checking which writer to use. `ShowResult` correctly uses `p.out`; errors should always go to `p.events` (stderr).

**Fix:** Single-line change: `p.writeJSON(p.out, ...)` → `p.writeJSON(p.events, ...)`.

### Bug 2: stepAugment and stepCoverLetter failures didn't set status = "degraded"
Both failure paths appended a `RiskWarning` but forgot `result.Status = "degraded"`. The spec explicitly states all non-fatal failures must set degraded status.

**Root cause:** `stepKeywords` was the reference implementation and it correctly set the status. When `stepAugment` and `stepCoverLetter` were added later, the pattern was copied incompletely — the warning append was copied but not the status assignment.

**Fix:** Added `result.Status = "degraded"` to both error paths.

### Bug 3: --headless flag was a no-op
The `--headless bool` flag was declared and bound but immediately suppressed with `_ = headlessMode`. Users could pass `--headless=false` and nothing would change.

**Fix:** Removed the flag entirely. The `Long` description explains JSON-only for this release. The flag will be added back when TUI mode is implemented.

---

## Process Notes

- Pre-review tests existed for keyword degradation and cover letter degradation but **did not assert `Status == "degraded"`** — they only checked for warnings. This let bug #2 slip past unit tests.
- **Lesson:** When writing degraded-mode tests, always assert both `len(result.Warnings) > 0` AND `result.Status == "degraded"`.
- Three commits needed after initial implementation (typed-nil fix, reviewer round 1, reviewer round 2). Could have been one if degraded-mode assertions had been stronger from the start.
