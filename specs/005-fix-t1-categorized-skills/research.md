# Research: T1 Category-Aware Skills Edits

**Branch**: `005-fix-t1-categorized-skills` | **Date**: 2026-04-25  
**Status**: Complete — no unknowns remain

## Decisions

### D1: Option A (category field) over Option B (flatten)

**Decision**: Add an optional `Category` field to `port.Edit`. Do NOT silently flatten categorized skills.

**Rationale**:
- Flattening is an implicit, lossy transformation — directly violates Constitution §IV (No Silent Failures)
- The existing error message in `apply_edits.go` ("use section-specific edits") is preserved design intent pointing toward a category field
- The orchestrator receives `sections.skills.categorized` from `submit_keywords`, giving it the category names needed to construct valid edits

**Alternatives considered**:
- Flatten categorized → flat on first T1 edit: rejected (lossy, silent mutation, violates constitution)
- Auto-create new categories on unknown category name: rejected (out of scope, introduces data model complexity)

---

### D2: Comma-split and trim for value parsing on categorized add/replace

**Decision**: The `value` field is a comma-separated string. The apply-edits layer parses it by splitting on `,`, trimming whitespace from each item, and operating on the resulting `[]string`.

**Rationale**:
- The categorized map stores `map[string][]string` — one entry per skill, not one string
- Appending the whole comma-separated string as a single list item would produce malformed rendering
- Consistent with how `add` and `replace` work conceptually: each comma-delimited token is a skill

---

### D3: category in both tool schema description and workflow prompt

**Decision**: Update both the `edits` parameter description in the `submit_tailor_t1` tool registration and the `workflowPromptText` in `prompt.go`.

**Rationale**:
- The MCP framework uses a `mcp.WithString("edits", mcp.Description(...))` description string — the inner JSON object schema is conveyed via this description, not via a structured schema
- Updating only the prompt text risks the orchestrator constructing edits without `category` if it infers the schema from the tool description alone
- Both surfaces must agree

---

### D4: Rejection on missing or unknown category (no auto-create, no silent ignore)

**Decision**:
- Missing `category` on a categorized section → reject with message listing available categories
- Unknown `category` name → reject with message listing available categories
- `category` on a flat section → silently ignored (backward compatible)

**Rationale**:
- Explicit rejection gives the orchestrator actionable feedback to retry with a valid category
- Listing available categories in the rejection reason removes the need for the orchestrator to re-read the sections payload
- Silently ignoring `category` on flat sections preserves backward compatibility without a new error code path

---

### D5: Worktree parallelization — two independent agents

**Decision**: Dispatch two parallel agents after confirming file ownership:
- **Agent A (Sonnet)**: port + service layers (`port/tailor.go`, `apply_edits.go`, `apply_edits_test.go`)
- **Agent B (Haiku)**: MCP adapter layer (`server.go` description string, `prompt.go` text)

**Rationale**:
- The two groups touch completely non-overlapping files — zero merge conflict risk
- Agent B's changes are pure string updates (no Go type references to `Category`) and can proceed in parallel
- Sonnet for Agent A: category routing logic requires careful logic (null checks, map iteration for error messages)
- Haiku for Agent B: description string updates are mechanical and low-risk
