# API Requirements Quality Checklist: T1 Skill Section Rewrites

**Purpose**: Validate the quality, completeness, clarity, and measurability of requirements for the `submit_tailor_t1` and `submit_keywords` contract changes — not whether the implementation works.
**Created**: 2026-04-23
**Feature**: [spec.md](../spec.md)
**Audience**: PR reviewer
**Depth**: Standard

---

## Requirement Completeness

- [x] CHK001 — Are JSON encoding rules for `skill_rewrites` (compact vs. pretty-printed) specified in the contract? [Gap, Contracts §submit_tailor_t1] → Resolved: any valid JSON accepted; server uses `json.Unmarshal` which is format-agnostic (Spec §Assumptions).
- [x] CHK002 — Are backwards-compatibility requirements defined for clients still sending the old `skill_adds` parameter? [Gap, Spec §FR-001] → Resolved: breaking change explicitly accepted; non-live internal service, no migration path required (Assumptions §5).
- [x] CHK003 — Are requirements defined for what `substitutions_made` returns when ALL `original` strings go unmatched? [Completeness, Spec §Edge Cases] → Resolved: returns 0 — entry-level count, each unmatched pair contributes 0 (Spec §Edge Cases).
- [x] CHK004 — Is a maximum length defined for `original` and `replacement` string values? [Gap] → Resolved: no hard server-side cap; FR-006 prompt guidance directs orchestrator to keep each `replacement` ≤40 chars longer than its target line to preserve single-page resume for ≤5 years experience.
- [x] CHK005 — Are requirements defined for `skills_section` when multiple resumes are scored but none contain a Skills section header? [Completeness, Spec §FR-004] → Resolved: FR-004 states field MUST be omitted or empty when no Skills section exists; applies regardless of how many resumes were scored.

---

## Requirement Clarity

- [x] CHK006 — Is "Skills section boundary" defined with a concrete detection rule (which header patterns are recognised as Skills headers)? [Clarity, Spec §FR-003] → Resolved: case-insensitive regex; optional Markdown heading prefix (`#`–`###`), optional adjective (`technical|core|key|professional|additional`), then "skills" + optional whitespace/colon. Documented in Spec §Assumptions and §Clarifications.
- [x] CHK007 — Is "verbatim text" in FR-004 clarified — does it include the section header line, trailing whitespace, or platform-specific line endings? [Clarity, Spec §FR-004] → Resolved: body text only (lines after the header line, header excluded). FR-004 and Key Entities updated; data-model.md updated.
- [x] CHK008 — Is the behaviour of `original` values with leading or trailing whitespace specified (trimmed before match, or exact)? [Clarity, Spec §Edge Cases] → Resolved: exact byte match, no trimming — mirrors `ApplyBulletRewrites`. Orchestrator has verbatim section text from US2. Documented in Spec §Edge Cases.
- [x] CHK009 — Is `substitutions_made` counting semantics explicit in the spec — entry-level count (max = len(rewrites)) vs. total occurrence count? [Clarity, Spec §Edge Cases + data-model.md] → Resolved: entry-level count explicitly stated in Spec §Edge Cases.
- [x] CHK010 — Is "prefer one-for-one swaps" in FR-006 defined with a measurable criterion, or is it aspirational guidance only? [Clarity, Spec §FR-006] → Resolved: testable per CHK033 — prompt MUST contain specific swap-preference language verifiable by inspecting the prompt string in tests.

---

## Requirement Consistency

- [x] CHK011 — Are the four error codes in the contract (`missing_skill_rewrites`, `invalid_skill_rewrites`, `empty_skill_rewrites`, `too_many_rewrites`) each traceable to a specific validation rule in FR-002 or FR-005? [Consistency, Contracts §Validation rules + Spec §FR-002/FR-005] → Resolved: contracts table maps each code to its condition; FR-002 covers missing/invalid/empty, FR-005 covers cap.
- [x] CHK012 — Does FR-001 ("replacing `skill_adds`") conflict with the Assumptions section stating CLI-mode T1 retains its existing interface? [Consistency, Spec §FR-001 + §Assumptions] → Resolved: no conflict — FR-001 is scoped to `submit_tailor_t1` (MCP handler); CLI-mode T1 (`PlanT1`/`SkillAdds`) is explicitly out of scope (Spec §Assumptions §3–4).
- [x] CHK013 — Is the `skills_section_found: false` response code consistent across all scenarios — does it appear for "Skills section missing" but NOT for "cap exceeded"? [Consistency, Spec §US1-Scenario3 + §Edge Cases] → Resolved: cap exceeded returns a validation error before any Skills section lookup; `skills_section_found: false` only appears when the section is absent from the resume.

---

## Acceptance Criteria Quality

- [x] CHK014 — Can SC-001 ("100% of T1-tailored resumes contain no uncategorized keyword line") be verified without human review? Is "uncategorized line" defined programmatically? [Measurability, Spec §SC-001] → Resolved: SC-002 (line count does not increase) serves as the automatable proxy for SC-001; equal line count before/after is the test assertion.
- [x] CHK015 — Does the reformulated SC-002 ("no bare-keyword lines") define "bare line" precisely enough to be automatable — e.g., a line with no colon (`:`) delimiter? [Measurability, Spec §SC-002] → Resolved: SC-002 reformulated to "line count does not increase after tailoring" — unambiguous and automatable without requiring colon-presence heuristics.
- [x] CHK016 — Is SC-005 ("replacements never modify text outside Skills section") backed by an explicit acceptance scenario in US1 where the same string appears in both Skills and Experience? [Measurability, Spec §SC-005 vs. US1-Scenario2] → Resolved: US1-Scenario2 explicitly uses "CI/CD" appearing in both Skills section and an Experience bullet.

---

## Scenario Coverage

- [x] CHK017 — Is a scenario defined for `original` that equals the full content of a Skills section line (replacing an entire line)? [Coverage, Gap] → Out of scope: general `strings.ReplaceAll` already handles this case; no special scenario needed. Covered implicitly by US1-Scenario1.
- [x] CHK018 — Are requirements defined for intentional shrink rewrites where `replacement` is shorter than `original` (skill removal or abbreviation)? [Coverage, Spec §Assumptions] → Resolved: no restriction on relative length; any non-empty replacement is accepted. SC-002 (line count) still passes since no line is added.
- [x] CHK019 — Is a scenario defined for flat (non-categorized) Skills section format — i.e., no `Label: items` structure, just a comma-separated list? [Coverage, Spec §Assumptions] → Resolved: Assumptions §2 explicitly states both categorized and flat formats are supported as long as `original` exists within the section.
- [x] CHK020 — Is a scenario defined for an empty `replacement` string, distinct from an empty `original` string? [Coverage, Spec §Edge Cases] → Resolved: empty `replacement` is valid (not blocked by server); effectively deletes the matched substring. Mirrors T2 `ApplyBulletRewrites` behaviour. Empty `original` is the only blocked case.

---

## Edge Case Coverage

- [x] CHK021 — Is behaviour defined when `skill_rewrites` is a valid JSON array but every entry has `original == ""`? Does this map to `empty_skill_rewrites` or succeeds with 0 substitutions? [Edge Case, Spec §Edge Cases + Contracts §Validation] → Resolved: returns `empty_skill_rewrites` error — after filtering, zero valid entries remaining is equivalent to an empty array (Spec §Edge Cases).
- [x] CHK022 — Is behaviour specified for `resumeText` that is an empty string or whitespace-only? [Edge Case, Gap] → Out of scope: `ExtractSkillsSection` returns `found=false` for empty input; handler proceeds normally with `skills_section_found: false`. No special handling required.
- [x] CHK023 — Are `original` values containing regex-special or language-specific characters (e.g., `C++`, `C#`, `.NET`) addressed? `strings.ReplaceAll` is literal-match but the spec should document the assumption. [Edge Case, Gap] → Out of scope: `strings.ReplaceAll` is a literal-string match; no regex interpretation. Documented in Spec §Edge Cases as assumption.
- [x] CHK024 — Is behaviour defined when `original` appears in both a category label AND its values (e.g., `Python: Python, Go`)? [Edge Case, Gap] → Out of scope: `strings.ReplaceAll` replaces all occurrences — both label and value occurrences are replaced. This is consistent with the "replace all occurrences" behaviour documented in Spec §Edge Cases.
- [x] CHK025 — Are explicit logging requirements defined for T1 rewrite operations — which fields (e.g., `substitutions_made`, `skills_section_found`, session ID) MUST appear in structured log output? [Coverage, Spec §Plan §Constitution Check V] → Resolved: T009 task description explicitly requires slog entry with `substitutions_made`, `skills_section_found`, and session ID; T011 requires slog for `skills_section` extraction result.
- [x] CHK026 — Is `MaxTier1SkillRewrites` specified as live-configurable or requiring a server restart to take effect? [Completeness, Gap] → Resolved: static config loaded at server startup from `defaults.json`; changing the value requires a server restart. Consistent with all other `TailorDefaults` config fields.
- [x] CHK027 — Are there requirements for sanitizing `skills_section` content before returning it (e.g., if the resume text contains PII or sensitive data)? [Coverage, Gap — flag if out of scope] → Out of scope: `submit_keywords` is an internal MCP tool; caller supplied the resume text, so returning the Skills section introduces no new exposure. No sanitization requirements.

---

## Dependencies & Assumptions

- [x] CHK028 — Is the assumption that `port.BulletRewrite` JSON tags produce `original`/`replacement` documented with a pointer to the type definition, so reviewers can verify without reading the port package? [Assumption, Spec §Assumptions + data-model.md] → Resolved: data-model.md §Reused Types documents the struct with JSON tags explicitly.
- [x] CHK029 — Is `loadBestResumeText` error behaviour (file not found, I/O error) specified as a dependency of the US2 `skills_section` feature? [Dependency, Spec §US2] → Resolved: Spec §Assumptions states that I/O failure MUST cause the handler to return an error (not silently omit the field).
- [x] CHK030 — Is the assumption that CLI-mode T1 users are unaffected by this change linked to a formal follow-up item (ticket, issue, or plan note)? [Assumption, Spec §Assumptions] → Resolved: Spec §Assumptions §3–4 explicitly states CLI-mode T1 is out of scope until a follow-up change; acceptable to leave as a documented assumption rather than a linked ticket for this internal service.

---

## Ambiguities & Conflicts

- [x] CHK031 — Is the ordering of rewrites specified? If two `original` strings overlap (e.g., `"CI/CD"` and `"CI/CD pipelines"`), is the outcome deterministic when applied in array order? [Ambiguity, Spec §US1 + research.md] → Resolved: FR-007 added — server applies in array order; orchestrator SHOULD order by expected ATS impact (highest-impact first); orchestrator owns overlap resolution.
- [x] CHK032 — Are all three "empty-ish" input states distinguished and mapped to separate error codes: absent parameter → `missing_skill_rewrites`, empty string parameter → `missing_skill_rewrites`, parsed-but-empty array → `empty_skill_rewrites`? [Ambiguity, Contracts §Validation rules] → Resolved: contracts validation table explicitly maps all three states to their respective error codes.
- [x] CHK033 — Does FR-006 ("MUST guide the orchestrator") create an acceptance-testable requirement (e.g., the prompt must contain a specific keyword or phrase) or is it untestable guidance? [Ambiguity, Spec §FR-006] → Resolved: testable — prompt MUST contain explicit swap-preference language; verifiable by inspecting the prompt string in tests (FR-006 updated).

---

## Notes

- Items marked `[Gap]` represent requirements not currently present in the spec — they need an explicit decision (add the requirement, or document as explicitly out of scope).
- Items marked `[Ambiguity]` can be resolved by adding a clarification bullet to `spec.md §Clarifications`.
- Items marked `[Conflict]` require one of the conflicting requirements to be updated before implementation begins.
- Check items off as resolved: `[x]`
