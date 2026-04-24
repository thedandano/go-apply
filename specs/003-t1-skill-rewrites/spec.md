# Feature Specification: T1 Skill Section Rewrites

**Feature Branch**: `003-t1-skill-rewrites`
**Created**: 2026-04-23
**Status**: Draft

## User Scenarios & Testing

### User Story 1 — Keywords land inside existing skill categories (Priority: P1)

A user applies to a job requiring Apache Kafka and Databricks. After T1 tailoring, those
keywords appear inside the appropriate existing category lines in the Skills section — not as
a new raw line at the bottom. The resume looks like a human edited it.

**Why this priority**: This is the core defect being fixed. The visible append artifact is the
primary reason for the feature.

**Independent Test**: Submit a `skill_rewrites` payload with one replacement targeting an
existing category line. Verify the Skills section contains the replacement inline and no new
uncategorized line was appended.

**Acceptance Scenarios**:

1. **Given** a resume with `Cloud & Infrastructure: AWS, Docker, Kubernetes`,
   **When** `skill_rewrites: [{"original": "Kubernetes", "replacement": "Kubernetes, EKS"}]` is submitted,
   **Then** the Skills section reads `Cloud & Infrastructure: AWS, Docker, Kubernetes, EKS` and
   no new uncategorized line is appended.

2. **Given** a resume with `Practices: CI/CD, TDD/BDD` and an Experience bullet also containing
   "CI/CD",
   **When** `skill_rewrites: [{"original": "CI/CD", "replacement": "Apache Kafka, CI/CD"}]` is submitted,
   **Then** the replacement applies only inside the Skills section; the Experience bullet is
   unchanged.

3. **Given** `skill_rewrites` with 6 items and `MaxTier1SkillRewrites = 5`,
   **When** the request is submitted,
   **Then** the server returns an error and the resume is unchanged.

---

### User Story 2 — Orchestrator receives Skills section to write accurate replacements (Priority: P2)

After scoring, the orchestrator receives the raw Skills section text alongside the score so it
can write precise `replace`/`with` pairs that exactly match the strings in the resume.

**Why this priority**: Without the Skills section text, the orchestrator must guess exact
formatting, commas, and spacing — leading to replacements that silently miss.

**Independent Test**: Call `submit_keywords` and verify the response includes a `skills_section`
field containing the resume's Skills section text verbatim.

**Acceptance Scenarios**:

1. **Given** a resume with a populated Skills section,
   **When** `submit_keywords` is called,
   **Then** the response includes `skills_section` with the verbatim text of that section.

2. **Given** a resume with no Skills section header,
   **When** `submit_keywords` is called,
   **Then** `skills_section` is absent or empty and no error is returned.

---

### User Story 3 — Length cap prevents the Skills section from bloating (Priority: P3)

The number of skill rewrites per T1 call is capped. Submitting more than the allowed number
is rejected with a clear error. The tool prompt guides the orchestrator to prefer one-for-one
swaps over pure appends.

**Why this priority**: Without a cap, repeated or over-eager orchestration could make the
Skills section unreasonably long, hurting readability and ATS compatibility.

**Independent Test**: Submit a request exceeding the cap and confirm the error code and that
the resume is unchanged.

**Acceptance Scenarios**:

1. **Given** `MaxTier1SkillRewrites = 5` (default),
   **When** a request with 5 or fewer rewrites is submitted,
   **Then** the request succeeds.

2. **Given** `MaxTier1SkillRewrites = 5`,
   **When** a request with 6 rewrites is submitted,
   **Then** the server returns error code `too_many_rewrites` and the resume is unchanged.

---

### Edge Cases

- What happens when `original` exists in the Skills section more than once?
  (All occurrences within the Skills section boundary are replaced — mirrors `ApplyBulletRewrites`.)
- What happens when an `original` string is not found in the Skills section?
  (Silently skipped; counted in `substitutions_made` as 0 for that entry; not an error.)
- What is counted in `substitutions_made`?
  (Entry-level count: each rewrite pair that matches at least once increments the count by 1,
  regardless of how many occurrences of `original` were replaced within the Skills section.
  Maximum value = len(skill_rewrites).)
- What happens when the Skills section is missing from the resume entirely?
  (All rewrites are skipped; `skills_section_found: false` is returned.)
- What happens when `original` is an empty string?
  (Entry is skipped; matches existing `ApplyBulletRewrites` behaviour.)
- What happens when all entries in `skill_rewrites` have an empty `original`?
  (After filtering empty-original entries, if no valid entries remain, the request is rejected
  with `empty_skill_rewrites` — equivalent to submitting an empty array.)
- How is `original` matched — is whitespace trimmed before comparison?
  (No — `original` is matched byte-exact against the Skills section text; no trimming is applied.
  This mirrors `ApplyBulletRewrites` behaviour. The orchestrator receives the verbatim section
  text via `skills_section` so it can supply the exact string.)
- Do `original` strings containing special characters (e.g., `C++`, `C#`, `.NET`) require
  special handling?
  (No — `strings.ReplaceAll` is a literal-string match; no regex interpretation occurs. All
  characters are matched as-is. The orchestrator must supply the exact bytes as they appear in
  the Skills section.)

## Requirements

### Functional Requirements

- **FR-001**: The `submit_tailor_t1` tool MUST accept `skill_rewrites` — a JSON array of
  `{original, replacement}` objects (same shape as `port.BulletRewrite`) — replacing the
  previous `skill_adds` string array.
- **FR-002**: An empty `skill_rewrites` array MUST be rejected with a validation error before
  any processing occurs.
- **FR-003**: Replacements MUST be applied only within the Skills section boundary. Text
  outside that boundary (e.g., Experience bullets) MUST NOT be modified.
- **FR-004**: The `submit_keywords` response MUST include a `skills_section` field containing
  the verbatim body text of the Skills section (lines after the header line, excluding the
  header line itself) from the best-scored resume. If no Skills section exists, the field
  MUST be omitted or empty.
- **FR-005**: A configurable integer cap (`MaxTier1SkillRewrites`, default 5) MUST exist.
  Requests exceeding the cap MUST be rejected before any substitution is attempted.
- **FR-006**: The `submit_tailor_t1` tool description and workflow prompt MUST reflect the
  `skill_rewrites` shape and MUST contain explicit "prefer one-for-one swaps" language
  (verifiable by inspecting the prompt string in tests). The prompt SHOULD also advise keeping
  replacements concise (each `replacement` ≤40 characters longer than its `original`) to
  preserve single-page resume length for candidates with ≤5 years experience.
- **FR-007**: Rewrites MUST be applied in submission array order. The orchestrator SHOULD
  order entries by expected ATS impact (highest-impact keyword first). When two `original`
  strings overlap (e.g., `"CI/CD"` and `"CI/CD pipelines"`), the orchestrator is responsible
  for ordering them to avoid unintended double-replacement; the server applies them as-is in
  array order.

### Key Entities

- **SkillRewrite**: An `{original, replacement}` pair (reuses `port.BulletRewrite`) specifying
  an exact string substitution scoped to the Skills section.
- **skills_section**: Verbatim body text of the resume's Skills section (lines after the header
  line, header excluded); returned by `submit_keywords` to give the orchestrator accurate context.
- **MaxTier1SkillRewrites**: Integer config cap on rewrites per T1 call; default 5.

## Success Criteria

### Measurable Outcomes

- **SC-001**: 100% of T1-tailored resumes contain no uncategorized keyword line appended at
  the bottom of the Skills section.
- **SC-002**: T1-tailored Skills section line count does not increase after tailoring —
  rewrites modify existing text in-place, not append new lines — verified by automated test
  asserting equal Skills section line count before and after `ApplySkillsRewrites`.
- **SC-003**: Requests exceeding the rewrite cap are rejected 100% of the time before any
  resume modification occurs.
- **SC-004**: `submit_keywords` returns the Skills section text in 100% of responses where a
  Skills section exists in the resume.
- **SC-005**: Replacements never modify text outside the Skills section, verified by tests
  where the `original` string appears in both Skills and Experience sections.

## Clarifications

### Session 2026-04-23

- Q: Should `skill_rewrites` use `replace`/`with` field names or `original`/`replacement` (matching `port.BulletRewrite`)? → A: `original`/`replacement` — reuse `port.BulletRewrite` as-is.
- Q: When `original` appears more than once in the Skills section, replace first match only or all occurrences? → A: Replace all occurrences within the Skills section boundary (mirrors `ApplyBulletRewrites`).
- Q: Is backwards compatibility with `skill_adds` required? → A: No — breaking change is acceptable; this is a non-live internal service with no external consumers.
- Q: Is there a length constraint on `original`/`replacement` strings? → A: No hard server-side cap; FR-006 prompt guidance directs the orchestrator to keep each `replacement` to ≤40 chars longer than its target line to preserve single-page resume length for candidates with ≤5 years experience.
- Q: When all entries have `original == ""`, should the request fail or silently succeed? → A: Fail with `empty_skill_rewrites` — after filtering empty-original entries, zero valid entries remaining is equivalent to an empty array.
- Q: When two `original` strings overlap, which takes priority? → A: Whichever improves ATS scoring — orchestrator orders by expected ATS impact; server applies in array order (FR-007).
- Q: Is FR-006 prompt guidance testable or aspirational? → A: Testable — the prompt MUST contain specific swap-preference language, verifiable by inspecting the prompt string in tests.
- Q: Does the `skill_rewrites` JSON require a specific format (compact vs. pretty-printed)? → A: Any valid JSON is accepted; server uses `json.Unmarshal` which is format-agnostic.
- Q: Which resume section headers are recognised as "Skills" by the server? → A: Matched by a case-insensitive regex: optional Markdown heading prefix (`#`–`###`), optional adjective (`technical|core|key|professional|additional`), then "skills" followed by optional whitespace or colon. E.g. `SKILLS`, `Technical Skills:`, `## Core Skills` all match.
- Q: Does `skills_section` include the header line (e.g. "SKILLS") or only the body? → A: Body text only — the header line is excluded. The returned text begins on the line immediately after the header.

## Assumptions

- The orchestrator is responsible for reasoning about which skills to add or swap and for
  supplying an `original` string that uniquely identifies the target within the Skills section.
- Resume Skills sections may be categorized (`Label: item, item`) or flat (`item, item`); the
  replacement works for both as long as the `original` string exists within the section.
- The Skills section header is detected by a case-insensitive regex that matches an optional
  Markdown heading prefix (`#`, `##`, `###`), an optional adjective (`technical`, `core`,
  `key`, `professional`, or `additional`), and the word `skills` (followed by optional
  whitespace or colon). Examples: `SKILLS`, `Technical Skills:`, `## Core Skills`.
- `skill_rewrites` payload MUST be a valid JSON array of `{original, replacement}` objects.
  Any valid JSON (compact or pretty-printed) is accepted; the server does not enforce formatting.
- If `loadBestResumeText` fails with an I/O error when `submit_keywords` tries to read the
  resume file, the handler MUST return an error to the caller rather than silently omitting
  the `skills_section` field.
- CLI-mode T1 (`PlanT1` / `SkillAdds`) is out of scope and remains on its existing interface
  until a follow-up change.
- `AddKeywordsToSkillsSection` may be retained for CLI mode; it is not deleted by this change.
- Backwards compatibility with `skill_adds` is explicitly not required; `submit_tailor_t1` is
  not a live service with external consumers, making a breaking input schema change acceptable.
