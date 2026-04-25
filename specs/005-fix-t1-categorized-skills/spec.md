# Feature Specification: T1 Category-Aware Skills Edits

**Feature Branch**: `005-fix-t1-categorized-skills`  
**Created**: 2026-04-24  
**Status**: Draft  
**Input**: Fix T1 skill edits silently rejected for categorized skills sections (Issue #109)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Apply T1 keyword edits on a categorized skills resume (Priority: P1)

A user has a resume with a categorized skills section (e.g., `Backend & Data: Go, PostgreSQL` / `AI & Emerging: Apache Kafka`). After scoring, they want to inject missing keywords into the correct category during T1 tailoring. Currently all edits are rejected with no score improvement.

**Why this priority**: This is the primary bug. Users with categorized skills (a common onboarding format) are completely blocked from T1 tailoring.

**Independent Test**: Onboard a resume with categorized skills, run through `submit_keywords`, call `submit_tailor_t1` with a category-targeted edit — verify the edit is applied and score improves.

**Acceptance Scenarios**:

1. **Given** a resume with `skills.kind = "categorized"` and categories `Backend & Data`, `AI & Emerging`, **When** `submit_tailor_t1` is called with `{"section":"skills","category":"Backend & Data","op":"add","value":"Apache Kafka, Spark"}`, **Then** the edit is applied, `edits_applied = 1`, and the score is recalculated against the updated resume text.
2. **Given** a resume with `skills.kind = "categorized"`, **When** `submit_tailor_t1` is called with `{"section":"skills","category":"Backend & Data","op":"replace","value":"PostgreSQL, Apache Kafka, Spark"}`, **Then** the entire `Backend & Data` category is replaced with the new value and the score is recalculated.
3. **Given** a resume with `skills.kind = "flat"`, **When** `submit_tailor_t1` is called with an edit that omits `category`, **Then** the edit is applied as before — flat resumes are unaffected by this change.

---

### User Story 2 - Clear rejection when category is missing or unknown (Priority: P1)

When the orchestrator calls `submit_tailor_t1` with a categorized skills resume but omits the `category` field (or uses a category name that doesn't exist), the system must reject with a precise, actionable error — not silently succeed, not corrupt the resume.

**Why this priority**: Without a clear rejection, the orchestrator could silently apply an edit to the wrong place or corrupt the discriminated union. Good error messages prevent incorrect LLM behavior on retries.

**Independent Test**: Call `submit_tailor_t1` on a categorized resume without a `category` field — verify rejection with a message that lists available categories.

**Acceptance Scenarios**:

1. **Given** a resume with `skills.kind = "categorized"` and category `Cloud`, **When** `submit_tailor_t1` is called with `{"section":"skills","op":"add","value":"AWS"}` (no `category`), **Then** the edit is rejected with a message indicating that `category` is required for categorized skills and listing the available category names.
2. **Given** a resume with `skills.kind = "categorized"` and category `Cloud`, **When** `submit_tailor_t1` is called with `{"section":"skills","category":"Nonexistent","op":"add","value":"AWS"}`, **Then** the edit is rejected with a message indicating the category does not exist and listing available categories.
3. **Given** a resume with `skills.kind = "flat"`, **When** `submit_tailor_t1` is called without `category`, **Then** the edit is applied normally — `category` is only required when the skills section is categorized.
4. **Given** a resume with `skills.kind = "categorized"` and category `Cloud`, **When** `submit_tailor_t1` is called with two edits — one valid (`category: "Cloud"`) and one invalid (unknown `category: "Nonexistent"`) — **Then** the valid edit is applied, `edits_applied = 1`, and `edits_rejected = 1` with the appropriate rejection reason. The call does not abort on the first invalid edit.

---

### User Story 3 - Workflow prompt guides the orchestrator to use category-targeted edits (Priority: P2)

The MCP workflow prompt must describe the `category` field so the LLM orchestrator knows how to construct correct T1 edits when the resume has a categorized skills section.

**Why this priority**: Without prompt guidance, the LLM will continue constructing edits without the `category` field and receive rejections. The schema fix alone is insufficient without corresponding prompt instructions.

**Independent Test**: Review the workflow prompt — verify it describes the `category` field and instructs the orchestrator to use `sections.skills.categorized` to determine available categories before constructing edits.

**Acceptance Scenarios**:

1. **Given** the workflow prompt is loaded, **When** the orchestrator reads step 5 (T1 tailoring), **Then** the prompt explicitly states that edits on `kind="categorized"` skills must include a `category` field matching a key from `sections.skills.categorized`.
2. **Given** the prompt's tool reference table, **When** `submit_tailor_t1` is described, **Then** the edit schema shows `{section:"skills", op, value, category?}` with a note that `category` is required for categorized skills.

---

### Edge Cases

- What happens when a user passes `category` on a flat skills section? → The `category` field is ignored; the flat edit proceeds normally.
- What happens when `categorized` map is empty (no categories yet)? → Edit with any category is rejected with a message noting no categories exist.
- What happens when `value` contains a single skill vs. a comma-separated list? → Both are accepted; the category's skill list is set or appended to accordingly.
- What happens when `op: remove` is used on skills? → Rejected — `remove` is not supported for skills (unchanged behavior).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The edit payload MUST accept an optional `category` field for skills section edits.
- **FR-002**: When the resume skills section is `kind = "categorized"` and a `category` is provided, the system MUST apply the edit to that specific category.
- **FR-003**: When the resume skills section is `kind = "categorized"` and no `category` is provided, the system MUST reject the edit with a message matching the template `"op %q on categorized skills requires a category; available: [%s]"`.
- **FR-004**: When the resume skills section is `kind = "categorized"` and `category` names a key not present in the categories map, the system MUST reject the edit with a message matching the template `"category %q not found; available: [%s]"`.
- **FR-005**: When `op = "add"` targets a categorized section, the system MUST parse `value` by comma, trim whitespace from each item, and append each item individually to the named category's skill list.
- **FR-006**: When `op = "replace"` targets a categorized section, the system MUST parse `value` by comma, trim whitespace from each item, and replace the named category's skill list with the resulting items.
- **FR-007**: The system MUST NOT silently convert a `categorized` skills section to `flat` as a fallback.
- **FR-008**: Rejections caused by a missing or unknown `category` field MUST include the sorted list of available category names in the rejection reason message.
- **FR-011**: When a single `submit_tailor_t1` call contains a mix of valid category-targeted edits and invalid (missing or unknown category) edits, the system MUST apply each edit independently — valid edits are applied to the resume sections and returned in `edits_applied`; invalid edits are collected in `edits_rejected` with their per-edit rejection reason. The call MUST NOT abort on the first invalid edit.
- **FR-009**: The `submit_tailor_t1` MCP tool JSON schema MUST expose `category` as an optional field within the `edits` array item definition, AND the workflow prompt MUST describe its usage for categorized skills sections.
- **FR-010**: Existing behavior for `kind = "flat"` skills edits MUST be preserved — no regressions.

### Key Entities

- **Edit**: A single mutation instruction. Gains an optional `category` field used when targeting a categorized skills section.
- **SkillsSection**: The discriminated union representing the resume's skills data (`kind: flat | categorized`). The `categorized` variant maps category names to skill lists.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user with a categorized skills resume can complete T1 tailoring with at least one edit applied in a single `submit_tailor_t1` call.
- **SC-002**: 100% of T1 edits on categorized skills without a valid `category` field are rejected with a message that includes the available category names — no silent no-ops.
- **SC-003**: All existing tests for flat skills edits pass without modification.
- **SC-004**: The `category` field appears in both the `submit_tailor_t1` MCP tool JSON schema and the workflow prompt's T1 edit schema description.
- **SC-005**: A categorized skills resume that goes through T1 tailoring still renders its skills as categorized text in the output resume — the categorized structure is preserved.

## Clarifications

### Session 2026-04-25

- Q: When `op = "add"` on a categorized section, should the comma-separated `value` be parsed into individual items or appended as a single string? → A: Parse by comma, trim whitespace, append each item individually to the category's skill list.
- Q: Should `category` appear in the `submit_tailor_t1` MCP tool JSON schema, or only in the workflow prompt text? → A: Add `category` to the tool JSON schema as an optional field in addition to updating the prompt text.
- Q: Does a single call with mixed valid+invalid category-targeted edits abort on first failure or apply edits independently? → A: Edits are applied independently (following existing per-edit contract); valid edits go to `edits_applied`, invalid to `edits_rejected`. Added FR-011 to capture this.
- Q: Do FR-003 (missing category) and FR-004 (unknown category) produce the same or distinct rejection messages? → A: Distinct templates. FR-003 uses `"op %q on categorized skills requires a category; available: [%s]"`; FR-004 uses `"category %q not found; available: [%s]"`. FR-008 narrowed to cover only these two paths.

## Assumptions

- The LLM orchestrator receives `sections.skills.categorized` from `submit_keywords` and can read available category names from it before constructing T1 edits.
- Creating new categories via T1 edits (supplying a `category` value not already in the map) is out of scope — only existing categories can be targeted. This keeps the change minimal and avoids data model complexity.
- `op: remove` on skills is not in scope for this fix — it remains unsupported for both flat and categorized sections.
- The `value` field for categorized `add`/`replace` edits is a comma-separated string (same as flat), not a JSON array. The apply-edits layer parses it by comma and trims whitespace from each item before updating the category's `[]string` list.
- No changes to the on-disk sidecar format are required — `SkillsSection` already stores `categorized` as `map[string][]string`.
