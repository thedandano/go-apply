# Implementation Checklist: T1 Category-Aware Skills Edits

**Purpose**: Requirements quality validation across spec + plan — covers behavioral contract, data model integrity, and orchestrator guidance. For both author self-check and peer PR review.
**Created**: 2026-04-25
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md) | [data-model.md](../data-model.md) | [contracts/submit_tailor_t1.md](../contracts/submit_tailor_t1.md)

---

## Requirement Completeness

- [ ] CHK001 — Does the spec define what happens when `value` is empty, whitespace-only, or results in zero non-empty tokens after comma-split (e.g., `"  ,  ,  "`)? [Completeness, Gap — Spec §FR-005, FR-006]
- [ ] CHK002 — Is there a requirement specifying whether duplicate skill entries within a category are permitted or rejected after an `add` op? [Completeness, Gap]
- [ ] CHK003 — Does the spec define a maximum number of skills that can be added to a single category in one `add` or `replace` edit? [Completeness, Gap]
- [ ] CHK004 — Is there a requirement covering the scenario where `sections.skills.categorized` is present in the `submit_keywords` response but contains an empty map (`{}`)? [Completeness, Gap — Spec §Edge Cases]
- [ ] CHK005 — Does the spec define requirements for multiple edits in a single T1 call that target different categories of the same categorized skills section? [Completeness, Gap — Spec §User Story 1]
- [x] CHK006 — Is there a requirement for the scenario where a single T1 call contains a mix of valid category-targeted edits and invalid (missing/unknown category) edits — specifically, does partial application happen or is the whole call rejected? [Completeness, Gap — Spec §FR-002] <!-- Resolved: FR-011 added; per-edit independence contract explicitly documented; User Story 2 scenario 4 added for mixed-call acceptance -->

---

## Requirement Clarity

- [ ] CHK007 — Is the exact format of the available-categories list in rejection messages specified (e.g., sorted alphabetically, comma-separated, quoted)? [Clarity — Spec §FR-008]
- [x] CHK008 — Do FR-003 ("no category provided") and FR-004 ("unknown category") produce the same rejection message or distinct messages? The spec does not explicitly distinguish them. [Clarity, Ambiguity — Spec §FR-003, FR-004] <!-- Resolved: FR-003 and FR-004 updated with distinct message templates; FR-008 narrowed to cover only missing/unknown category paths -->
- [ ] CHK009 — Is SC-005 ("categorized structure is preserved") specific enough to define what "preserved" means — same category keys, same item order within each category, unchanged categories not targeted by the edit? [Clarity — Spec §SC-005]
- [ ] CHK010 — Is SC-001 ("at least one edit applied in a single call") specific enough to cover partial success (some edits applied, some rejected within the same call)? [Clarity — Spec §SC-001]
- [ ] CHK011 — Can "category field is silently ignored" (Edge Case 1, flat skills + category present) be objectively measured — is there a success criterion or FR that covers this case? [Measurability, Gap — Spec §Edge Cases]

---

## Behavioral Contract Requirements

- [ ] CHK012 — Is the comma-split + whitespace-trim contract for `value` parsing specified in the same place for both `add` and `replace` ops, or only in the Clarifications/Assumptions section where it may be missed? [Consistency — Spec §FR-005, FR-006, Clarifications]
- [ ] CHK013 — Does the contract distinguish whether comma-split parsing is done at the service layer (apply_edits.go) or at the MCP handler layer? [Clarity — data-model.md §applySkillsEdit, Plan §Agent A]
- [ ] CHK014 — Is FR-007 ("MUST NOT silently convert categorized to flat") accompanied by a success criterion that verifies this invariant rather than just stating it as a negative constraint? [Measurability, Gap — Spec §FR-007, SC-005]
- [ ] CHK015 — Are the rejection message strings in data-model.md (e.g., `"op %q on categorized skills requires a category; available: [%s]"`) consistent with what the spec describes in FR-003/FR-008? [Consistency — data-model.md §rejection message format, Spec §FR-003, FR-008]

---

## Data Model Integrity Requirements

- [ ] CHK016 — Does the spec or data-model.md state the invariant that edits to one category must NOT affect the contents of other categories in the same map? [Completeness, Gap — data-model.md §State Transitions]
- [ ] CHK017 — Is the deep-copy requirement for the categorized map (ensuring `ApplyEdits` does not mutate the caller's sections) referenced in the spec requirements, or is it only implied by the existing `copySections` helper? [Completeness, Gap — data-model.md §Unchanged Entities]
- [ ] CHK018 — Does the data-model.md specify the behavior of `replace` on a category whose current value is an empty `[]string` — is an empty result valid? [Completeness, Gap — data-model.md §State Transitions]
- [ ] CHK019 — Is the discriminated union invariant (if `Kind = "categorized"`, `Flat` must remain empty; if `Kind = "flat"`, `Categorized` must remain nil) formally stated in requirements rather than only in the existing test? [Completeness, Gap — Spec §FR-007]

---

## Orchestrator Guidance Requirements

- [ ] CHK020 — Does SC-004 ("category appears in both the tool JSON schema description and the workflow prompt") distinguish between which surface (tool description vs. prompt step 5 vs. prompt table) must contain the field, or is it ambiguous? [Clarity — Spec §SC-004, contracts/submit_tailor_t1.md]
- [ ] CHK021 — Is there a requirement defining what the orchestrator should do when `submit_keywords` returns `skills_section.kind = "categorized"` but `sections.skills.categorized` is absent or null? [Completeness, Gap — Spec §Assumptions]
- [ ] CHK022 — Does the prompt change requirement (FR-009 + contracts) specify whether the example in the tool description must show both flat AND categorized formats side by side, or only one? [Clarity — Spec §FR-009, contracts/submit_tailor_t1.md §Tool Registration]
- [ ] CHK023 — Are the workflow prompt instructions for T1 (Step 5) consistent with the tool reference table row for `submit_tailor_t1` — both must include `category?` with the same semantics? [Consistency — contracts/submit_tailor_t1.md §Workflow Prompt]

---

## Scenario & Edge Case Coverage

- [ ] CHK024 — Is there a requirement or acceptance scenario for the regression path where a flat-skills resume receives a T1 edit WITH a `category` field — verifying it is silently ignored and does not produce unexpected behavior? [Coverage — Spec §Edge Cases, §User Story 1 Scenario 3]
- [ ] CHK025 — Are requirements defined for the case where `op = "add"` is called multiple times on the same category within a single session — verifying skills accumulate correctly across calls? [Coverage, Gap]
- [ ] CHK026 — Is there a scenario requirement covering what happens when `sections.skills` is nil at the time of a categorized edit (the nil-init branch in `applySkillsEdit`)? [Coverage, Gap — data-model.md §applySkillsEdit]

---

## Plan Implementation Notes Quality (Agent A & B)

- [ ] CHK027 — Do the plan's Agent A notes specify the exact function signatures and return types for the `sortedKeys` and `splitTrim` helpers, or is their interface left to Agent A's discretion? [Completeness — Plan §Agent A Implementation Notes]
- [ ] CHK028 — Are the 7 new test cases in the plan's Agent A notes described with enough specificity (input sections, edit payload, expected EditsApplied/EditsRejected counts, expected category contents) that Agent A can implement them without guessing at assertions? [Completeness — Plan §Agent A, apply_edits_test.go]
- [x] CHK029 — Does the plan identify by name which existing test in `apply_edits_test.go` must be modified (the "categorized kind rejects flat ops" test), what the old assertion was, and what the new assertion must be? [Clarity — Plan §Agent A Implementation Notes] <!-- Resolved: Plan now specifies exact rename ("skills categorized rejects ops with missing category"), assertion change (contains "requires a category" AND "available:"), and adds second test ("skills categorized rejects ops with unknown category") with Category: "Nonexistent" assertion -->
- [ ] CHK030 — Do the plan's Agent B notes specify whether the `mcp.WithDescription` tool-level description (the `mcp.NewTool(...)` first string arg) also needs updating, or only the `edits` parameter description? [Clarity — Plan §Agent B Implementation Notes]
- [ ] CHK031 — Are the exact string changes for `prompt.go` in Agent B's notes precise enough that Agent B can implement them without needing to re-read the full `workflowPromptText` constant? [Completeness — Plan §Agent B, contracts/submit_tailor_t1.md §Workflow Prompt]

---

## Acceptance Criteria Measurability

- [ ] CHK032 — Can SC-002 ("100% of edits without valid category are rejected with available categories in message") be objectively verified without inspecting the exact rejection message string — is the message format requirement sufficiently specified? [Measurability — Spec §SC-002, FR-008]
- [ ] CHK033 — Is SC-003 ("all existing tests for flat skills edits pass") an adequate regression guard, or does it omit the MCP layer tests for flat resumes that would also be affected? [Completeness — Spec §SC-003]

---

## Assumptions & Dependencies

- [ ] CHK034 — Is the assumption "the LLM orchestrator reads `sections.skills.categorized` from `submit_keywords` before constructing T1 edits" validated against the actual `submit_keywords` response payload format (i.e., is `sections.skills.categorized` always present in the response when `kind = "categorized"`)? [Assumption — Spec §Assumptions]
- [ ] CHK035 — Is the assumption "no changes to the on-disk sidecar format are required" validated against the `SaveSections` call in the T1 handler — specifically, will the `map[string][]string` serialize and deserialize correctly after a categorized edit? [Assumption — Spec §Assumptions, Plan §Technical Context]
- [ ] CHK036 — Is the "no new-category creation" scope exclusion documented in both the spec AND the plan's implementation notes, so Agent A does not inadvertently implement auto-create behavior? [Consistency — Spec §Assumptions, Plan §Agent A]

---

## Notes

- Items marked `[Gap]` indicate a potential missing requirement — the author should decide if it needs to be added to the spec before implementation starts.
- Items marked `[Ambiguity]` indicate an underspecified requirement — the reviewer should ask the author to clarify before approving.
- Items marked `[Consistency]` flag potential contradictions between two documents — both should be reconciled.
- Check off items as reviewed: `[x]`
- Add inline findings: `- [ ] CHK### ... <!-- Finding: ... -->`
