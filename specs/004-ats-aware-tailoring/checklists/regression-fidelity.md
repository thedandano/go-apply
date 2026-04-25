# Regression & Conversation-Fidelity Checklist: ATS-Aware Resume Tailoring

**Purpose**: Unit-test the *requirements* (spec + plan + research + contracts + data-model + quickstart) for two things: (1) do they faithfully capture every decision reached in the conversation that preceded this spec, and (2) do they explicitly name regression guards for behavior that existed before this feature shipped.
**Created**: 2026-04-24
**Feature**: [spec.md](../spec.md)

## Conversation Fidelity — Clarifications Q1–Q4

- [x] CHK001 Are the four recorded clarifications (MCP+Headless scope, unified edit envelope, re-parse policy, dynamic renderer order) each traceable to at least one Functional Requirement? [Traceability, Spec §Clarifications]
- [x] CHK002 Is the MCP-vs-Headless division of labor for sections parsing specified in FRs, not only in prose? [Completeness, Spec §FR-001, FR-014]
- [x] CHK003 Is "TUI out of scope / continues on old regex path" stated consistently in spec AND plan (no version where TUI is implicitly included in the new path)? [Consistency, Spec §Out of Scope, Plan §Technical Context]
- [x] CHK004 Is the unified edit envelope shape `{section, op, target?, value?}` documented identically in spec, data-model.md, and contracts/mcp-tools.md (no drift between the three)? [Consistency, Spec §FR-004, Data-model §EditEnvelope, Contracts §4]
- [x] CHK005 Is the bullet-ID format `exp-<entry_index>-b<bullet_index>` specified with a concrete example (e.g., `exp-0-b2`) in each artifact that references bullet targeting? [Clarity, Spec §FR-005a]
- [x] CHK006 Is the re-parse policy written with mode-specific behavior for BOTH MCP (typed error with `raw`) AND Headless (automatic LLM call), not just one? [Completeness, Spec §FR-014]
- [x] CHK007 Are the YoE tiers (≥3 experienced, <3 entry-level) documented with the exact default section orderings, not paraphrased? [Clarity, Spec §FR-011]
- [x] CHK008 Is the orchestrator `order` override documented with its interaction rule (verbatim use, skip tier selection, handle missing/extra keys)? [Completeness, Spec §FR-011a]
- [x] CHK009 Are canonical heading labels enumerated (all 10: Contact, Summary, Work Experience, …) in at least one artifact, not just referenced by name? [Completeness, Spec §FR-011b]

## Conversation Fidelity — Alias Scoring

- [x] CHK010 Are the five alias pairs discussed in the conversation (Apache Spark↔PySpark, PostgreSQL↔Postgres, Kubernetes↔K8s, JavaScript↔JS, TypeScript↔TS) each enumerated in the spec, not left generic? [Completeness, Spec §FR-007]
- [x] CHK011 Is the bidirectional expansion property specified (either direction matches — matches from JD→resume and resume→JD)? [Clarity, Spec §Edge Cases]
- [x] CHK012 Is the no-false-positive guarantee for word-boundary tokens (`C++`, `.NET`, `Go`) written as an explicit requirement, not just implied? [Completeness, Spec §FR-008]
- [x] CHK013 Is the alias set size bound (≤20 pairs per assumption) consistent with the deferred config-override story (FR-D05)? [Consistency, Spec §Assumptions, Spec §FR-D05]
- [x] CHK014 Is alias-case-sensitivity behavior defined (case-insensitive lookup per data-model §AliasSet) in a requirement the user can point to? [Clarity, Data-model §AliasSet]

## Conversation Fidelity — Schema Tiers

- [x] CHK015 Are Tier 1 required keys (`contact`, `experience`) explicitly separated from Tier 2/3 optional keys in the FR text, not mixed? [Clarity, Spec §FR-003]
- [x] CHK016 Are Tier 4 keys (languages, speaking, open_source, patents, interests, references) listed as *deferred* with a rationale, not silently dropped? [Completeness, Spec §FR-D06]
- [x] CHK017 Is the `SchemaVersion = 1` constraint present at every boundary that persists sections (sidecar write, onboard_user, add_resume)? [Consistency, Data-model §SectionMap, Contracts §1–2]
- [x] CHK018 Is schema-version mismatch defined with a specific error code (`sections_unsupported_schema`) rather than a generic error? [Clarity, Contracts §Typed error codes]

## Conversation Fidelity — Renderer & Extractor Seam

- [x] CHK019 Is the Renderer interface scope (`Render(*SectionMap) (string, error)`) specified once and referenced everywhere, with no conflicting signature elsewhere? [Consistency, Contracts §1, Data-model §Renderer]
- [x] CHK020 Is the Extractor identity contract (`Extract(s) == s`) stated as a testable invariant, not only as a behavior hint? [Measurability, Data-model §Extractor, Contracts §2]
- [x] CHK021 Is `preview_ats_extraction` documented as a new MCP tool with an input schema and success payload, not just mentioned? [Completeness, Contracts §5]
- [x] CHK022 Is the forward-compatibility claim ("no caller change when real PDF pipeline is plugged in") written as a requirement the maintainer can verify? [Measurability, Spec §SC-004]

## Regression Prevention — Pre-Feature Behavior

- [x] CHK023 Are the three known failure modes from the PlayStation run (silent T1 fail, missing skills in response, Spark↔PySpark miss) each mapped to a specific FR that prevents recurrence? [Traceability, Spec §Background, Spec §US1–2]
- [x] CHK024 Are pre-feature resumes (raw-only records) specified to load without panic in BOTH modes, not just one? [Completeness, Spec §FR-014]
- [x] CHK025 Is "no silent data loss on migration" stated as a measurable success criterion, not only as a principle? [Measurability, Spec §SC-005]
- [x] CHK026 Are the exact deletions (`skillsHeaderRe`, `ApplySkillsRewrites`, `ExtractSkillsSection`) enumerated in the plan so nothing silently lingers? [Completeness, Plan §Project Structure]
- [x] CHK027 Is TUI mode's continued use of the old regex path specified as an explicit *carve-out*, not just "TUI out of scope"? [Clarity, Spec §Out of Scope, Spec §Assumptions]
- [x] CHK028 Are word-boundary tokens (`C++`, `.NET`, `Go`) called out as preservation targets in the scorer FR, not only implicit in the "no false positives" language? [Completeness, Spec §FR-008]

## Regression Prevention — API Surface

- [x] CHK029 Is every breaking MCP envelope change (removed `skills_section`, added `sections`, added `schema_version`) documented with a before/after in contracts, not just "new field"? [Completeness, Contracts §3]
- [x] CHK030 Is the rename from `skill_rewrites`/`bullet_rewrites` → `edits` specified with its removal status (removed, not coexisting)? [Consistency, Contracts §4]
- [x] CHK031 Is `port.Tailor.TailorResume` marked for removal at feature end (not left as deprecated dead code indefinitely)? [Clarity, Contracts §5]
- [x] CHK032 Are the new typed error codes (`missing_sections`, `invalid_sections`, `sections_missing`, `too_many_edits`, `invalid_edits`, `sections_unsupported_schema`) all distinct and each tied to a specific failure path? [Completeness, Contracts §Error envelope]

## Acceptance Criteria Measurability

- [x] CHK033 Can SC-001 ("zero silent substitutions") be objectively verified without inspecting implementation internals? [Measurability, Spec §SC-001]
- [x] CHK034 Can SC-002 ("credits matches at the same rate as canonical") be measured with a concrete test (e.g., a named JD × resume pair)? [Measurability, Spec §SC-002]
- [x] CHK035 Is SC-004 ("maintainer adds new renderer without modifying scoring/tailoring/MCP") paired with a test plan or quickstart scenario that confirms it? [Measurability, Spec §SC-004, Quickstart §4]
- [x] CHK036 Is SC-006 (PlayStation replay) specified with enough detail (JD identity, resume identity, expected match) that it can be rerun? [Clarity, Spec §SC-006, Quickstart §11]

## Scenario Coverage — Alternate & Exception Flows

- [x] CHK037 Is the `order` override flow covered by a named acceptance scenario or quickstart step, not only the happy-path default tiers? [Coverage, Quickstart §6]
- [x] CHK038 Is the sections-missing exception flow covered for MCP (typed envelope) AND for Headless (auto-parse), each with a distinct scenario? [Coverage, Quickstart §7]
- [x] CHK039 Is a malformed-target rejection scenario (e.g., `target: "exp-99-b0"`) specified with the expected error message shape? [Coverage, Quickstart §4]
- [x] CHK040 Are partial-success semantics (some edits apply, some rejected, envelope returns both lists) specified as a requirement rather than just a wire format? [Completeness, Spec §FR-005, Contracts §4]

## Ambiguities & Unresolved

- [x] CHK041 Is "student signal" (<3 YoE tier trigger) defined well enough that two reviewers would agree on when it fires? [Ambiguity, Spec §FR-011]
- [x] CHK042 Is the `dataDir/inputs/<label>.sections.json` naming convention captured in a requirement or design doc, or does it live only in research.md? [Completeness, Research §R3]
- [x] CHK043 Are concurrency constraints for the sections sidecar (atomic rename, single writer) specified, or left implicit? [Coverage, Research §R3]
- [x] CHK044 Is "force re-parse" scope in Headless quantified (one LLM call per record per upgrade, not per operation)? [Clarity, Spec §Assumptions, §FR-014]
- [x] CHK045 Does the spec name an owner or trigger for updating the hardcoded alias dictionary as technology names evolve? [Dependency, Spec §Dependencies]

## Non-Functional & Observability

- [x] CHK046 Are structured-log requirements (operation name, session ID, outcome, elapsed time) specified for every new MCP handler, not only stated as a principle? [Completeness, Plan §Constitution Check V]
- [x] CHK047 Is verbose-mode debug output (parsed SectionMap, pre/post-edit diff, alias expansions) specified as an observability requirement? [Completeness, Plan §Constitution Check V]
- [x] CHK048 Are ≥80% coverage expectations bound to specific packages (`internal/model`, `internal/service/{render,extract,scorer,tailor,orchestrator}`, `internal/repository/fs`, `internal/mcpserver`)? [Measurability, Plan §Testing]

## Notes

- Items with `[Gap]` flag likely-missing requirements. Items with `[Ambiguity]` flag phrases a reviewer would interpret two ways.
- Minimum 80% of items carry a spec/plan/research/contract reference — validate traceability by scanning `[Spec §…]` / `[Plan §…]` / `[Research §…]` / `[Contracts §…]` / `[Data-model §…]` / `[Quickstart §…]` counts.
- Cross-check before marking Phase 2 (`/speckit-tasks`) ready: CHK004, CHK017, CHK019, CHK029 — any inconsistency between the artifacts surfaces here first.
