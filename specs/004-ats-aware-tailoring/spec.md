# Feature Specification: ATS-Aware Resume Tailoring

**Feature Branch**: `004-ats-aware-tailoring`
**Created**: 2026-04-24
**Status**: Draft
**Input**: User description: "Structured resume sections model, alias-aware keyword scoring, and Renderer/Extractor interface seam. Also specs deferred work (real PDF rendering/extraction, layout survival warnings, schema expansion, config-driven aliases) so it's captured now while context is fresh."

## Clarifications

### Session 2026-04-24

- Q: Which go-apply modes are in scope for sections-based parsing? → A: MCP + Headless (CLI) in scope. TUI is on a deprecation path and is out of scope for this feature. Headless mode uses the existing internal LLM service to parse raw text into sections at onboarding; MCP mode receives sections from the orchestrating AI.
- Q: How should tailoring steps specify their edits now that sections are structured? → A: Unified edit envelope for both T1 and T2 (same input/output shape). Envelope: `{ edits: [{ section, op, target?, value? }] }` where `op` is `add`/`remove`/`replace`. Targets are strings for skills and stable bullet IDs for experience bullets. Bullet IDs use the format `exp-<entry_index>-b<bullet_index>` (e.g., `exp-0-b2`); can migrate to UUIDs later if positional IDs cause collisions. T1 and T2 tool names are retained for now as aliases of the same underlying function; future unification into a single `submit_tailor` tool requires no API change.
- Q: How should the system handle resumes onboarded before this feature shipped? → A: Force re-parse on next use. In Headless (CLI) mode, the internal LLM service is invoked automatically to produce sections from the stored raw text when a pre-feature record is loaded. In MCP mode, operations that require sections (scoring, tailoring) return a structured "sections missing — call add_resume with sections" error to the orchestrator, prompting re-onboarding. No mixed-mode execution: every resume eventually has a sections representation, and operations that depend on it fail loudly if it is absent.
- Q: In what order should the Renderer emit sections, and what headings should it use? → A: Dynamic, experience-forward default (Option D). The Renderer picks an order tier based on the YoE signal derivable from `sections.experience` (summed duration across entries). Default tier (≥3 YoE): `Contact → Summary → Experience → Skills → Education → Projects → Certifications → Awards → Volunteer → Publications`. Entry-level tier (<3 YoE, no experience entries, or student signal): `Contact → Summary → Education → Projects → Skills → Experience → Certifications → Awards → Volunteer → Publications`. The sections payload MAY include an optional `order: [string]` field; when present, the Renderer uses it verbatim and skips tier selection (edge cases — career changer, PhD, bootcamp grad — are handled by the orchestrator, not the Renderer). Independent of ordering, the Renderer MUST emit canonical heading labels (`Work Experience`, `Education`, `Skills`, `Projects`, `Certifications`, `Awards`, `Volunteer Experience`, `Publications`, `Summary`, `Contact`) regardless of the input's original labels, because ATS parsers key off recognized labels for section classification.

## Background

A live run of the go-apply MCP tool surfaced three failures during a job application to PlayStation's Data Platform team:

1. Skills-section tailoring silently did nothing because the user's heading was "Skills & Abilities" — a format the tool's text-parsing logic did not recognize.
2. The response to the keyword-scoring step omitted the skills-section content it was supposed to return, so the orchestrating AI had to guess at what to rewrite.
3. The scorer treated "PySpark" and "Apache Spark" as different keywords, penalizing a resume that clearly listed the required technology under a common alias.

These are not three bugs — they are one architectural mistake. The tool tries to understand resume structure by pattern-matching headings against an arbitrarily formatted text blob. Resumes come in too many shapes for this to ever be reliable.

This feature shifts resume-structure understanding from the tool to the orchestrating AI: the AI parses the resume into a structured sections map once (at onboarding), and all downstream operations (scoring, tailoring, cover-letter generation, final output) operate on that structured form. The feature also adds alias-aware keyword matching to catch obvious synonyms, and sets up an architectural seam for a future phase that will score against what the ATS actually sees, not what the tool assumes it sees.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Tailoring works regardless of resume heading style (Priority: P1)

A job seeker onboards their resume, which uses "Skills & Abilities" (or "Technical Stack", or any other variation) as the heading for their technical skills. They kick off an application to a role that requires skills they don't currently list. The tool successfully locates and edits their skills, producing a tailored resume.

**Why this priority**: Today this silently fails. The tool reports a tailored resume but makes zero changes. The user is misled into thinking their resume was improved when it wasn't. This is the single most damaging failure mode — it undermines trust in every score the tool produces.

**Independent Test**: Onboard a resume with a non-standard skills heading. Run the scoring + tailoring flow. Verify that tailoring edits the intended section and the rewritten content is reflected in the final output.

**Acceptance Scenarios**:

1. **Given** a resume where the skills section is titled "Skills & Abilities", **When** the job seeker applies to a role requiring a skill not listed on the resume, **Then** the tailoring step successfully adds the skill to the skills section and the final resume reflects that change.
2. **Given** a resume with no explicit skills heading at all (skills embedded in the summary), **When** the AI parses the resume at onboarding, **Then** the parsed representation still correctly identifies which content is "skills" for downstream tailoring.
3. **Given** a resume uploaded before this feature shipped, **When** the user begins a new job application, **Then** the tool either uses the existing data or prompts for re-onboarding, without crashing or producing a broken resume.

---

### User Story 2 — Synonyms don't penalize candidates (Priority: P1)

A job seeker's resume lists "PySpark" under their skills. They apply to a job description that requires "Apache Spark". The scorer recognizes these as the same technology and credits the match.

**Why this priority**: This is a correctness issue with immediate user impact. Every day the tool is in use, candidates are being penalized for writing their resume in industry-standard shorthand. The fix is small in scope but immediately improves score fidelity for every user.

**Independent Test**: Score a resume containing "PySpark" against a JD requiring "Apache Spark". Verify Apache Spark appears in `req_matched` and the total score reflects the match.

**Acceptance Scenarios**:

1. **Given** a resume listing "PySpark" and a JD requiring "Apache Spark", **When** scoring runs, **Then** the required keyword is marked as matched.
2. **Given** a resume listing "Postgres" and a JD requiring "PostgreSQL" (or vice versa), **When** scoring runs, **Then** the required keyword is marked as matched.
3. **Given** a resume listing "Go" and a JD requiring "JavaScript", **When** scoring runs, **Then** "Go" does not incorrectly match "JavaScript" via the alias system (no false positives).

---

### User Story 3 — AI orchestrator can see what it's working with (Priority: P2)

The orchestrating AI calls the scoring step and, in the response, receives the full structured representation of the resume it just scored. When it plans a tailoring action, it can reason about what's actually in each section instead of guessing.

**Why this priority**: Enables high-quality tailoring decisions. Without structure visibility, the AI has to make guesses about exact text to replace, which is what caused the silent failure in Story 1. Structure visibility makes the tailoring step deterministic and debuggable.

**Independent Test**: Call the scoring step after onboarding. Verify the response contains the full sections map (summary, skills, experience, education, etc.) with current content.

**Acceptance Scenarios**:

1. **Given** a resume has been onboarded, **When** the AI calls the scoring step, **Then** the response includes each populated section as a named field with its current content.
2. **Given** the AI proposes a tailoring edit to the skills section, **When** it passes that edit to the tailoring step, **Then** the edit lands exactly on the skills section with no ambiguity about targeting.

---

### User Story 4 — Architectural seam ready for ATS-accurate scoring (Priority: P2)

The codebase exposes a clean boundary between "render the resume for output" and "extract the text that scoring operates on". Today both are pass-through operations over text. Tomorrow, when the tool starts producing actual PDF files, this boundary is where real rendering and real extraction plug in — with no changes to scoring, tailoring, or the MCP surface.

**Why this priority**: This is an enabler, not a user-facing behavior. Its value is preserving a clear upgrade path. Without it, adding real rendering later would require unwinding assumptions baked into every downstream stage.

**Independent Test**: Inspect the code and confirm that a single `Renderer` interface and a single `Extractor` interface are used everywhere that text-for-scoring is produced. Confirm a new MCP tool exists that exposes the extracted text, even if today it is identical to the rendered text.

**Acceptance Scenarios**:

1. **Given** the architecture is in place, **When** a maintainer later swaps the default renderer for a PDF renderer, **Then** no scoring, tailoring, or MCP-envelope code needs to change.
2. **Given** an AI orchestrator wants to see "what the ATS would see" for the current resume, **When** it calls the ATS-preview tool, **Then** it receives the extractor's output (identity today, real extraction later).

---

### User Story 5 — Real ATS extraction catches layout-induced keyword loss (Priority: P3, DEFERRED)

A job seeker exports their resume as a PDF with a multi-column layout. The tool renders the PDF, runs an ATS-equivalent extractor over it, and warns the user that "PySpark" in their source content was lost in extraction because of column ordering. They can see the gap between their authored resume and what an ATS will actually read.

**Why this priority**: Closes the honesty gap in scoring — today's scoring lies when the final output has layout issues. Deferred because it requires a real PDF rendering pipeline (font support, templates, third-party dependencies) that has its own design space.

**Independent Test**: Render a resume as a known-problematic multi-column PDF. Run the extraction step. Verify the tool surfaces keyword-survival diffs between source and extracted text.

**Acceptance Scenarios**:

1. **Given** the renderer is set to produce a multi-column PDF, **When** the extractor runs over it, **Then** the tool reports which keywords present in the source were not recovered in extraction.
2. **Given** the renderer is set to produce an ATS-safe single-column format, **When** the extractor runs over it, **Then** all source keywords survive extraction.

**Status**: Deferred to a later feature. Requires resolving template choice, PDF library choice, and external extractor dependency.

---

### User Story 6 — Full-fidelity resume schema for unusual candidates (Priority: P3, DEFERRED)

A job seeker with a research background lists publications, patents, and speaking engagements on their resume. The tool parses and preserves all of these sections through every stage of the pipeline, so the final output isn't missing content that was on the source resume.

**Why this priority**: The common-case schema (contact, experience, summary, skills, education, projects, certifications, awards, volunteer, publications) already handles most real resumes. Rare sections (languages, speaking, open-source work, patents, interests, references) can be added when a real user request surfaces. No sense defining them speculatively.

**Independent Test**: Onboard a resume that includes one of the rare sections. Verify the section is preserved through scoring, tailoring, and final output.

**Acceptance Scenarios**:

1. **Given** a candidate has a "Speaking Engagements" section, **When** they onboard their resume, **Then** that section is preserved in the structured representation and shows up in the final output.

**Status**: Deferred. Schema extension is mechanical; prioritize when real user demand appears.

---

### User Story 7 — Users can customize the alias dictionary (Priority: P3, DEFERRED)

A job seeker working in a specialized domain (e.g., biotech, finance) adds domain-specific aliases to a configuration file so the scorer recognizes, for example, "R" and "R programming" as the same keyword, or their employer's internal tool name as equivalent to its public counterpart.

**Why this priority**: The built-in alias dictionary covers the top-N industry-standard synonyms. Configurability adds flexibility at the cost of surface area. Ship the baseline first; extend to config once the baseline's limits are visible from real usage.

**Independent Test**: Add a custom alias via configuration. Score a resume that uses the alias against a JD that uses the canonical form. Verify the match is credited.

**Status**: Deferred. Design the config surface only once the hardcoded default has been in use and its gaps are known.

---

### User Story 8 — ATS-specific extraction profiles (Priority: P3, DEFERRED)

A job seeker knows their target employer uses Workday (or Greenhouse, or Taleo). The tool runs extraction through a profile matching that specific ATS's known behavior, producing a more accurate preview than a generic extraction would.

**Why this priority**: Maximum fidelity for the honest-scoring story. Deferred because it requires the base extraction feature (Story 5) to ship first and requires accumulating behavioral knowledge about each ATS.

**Status**: Deferred. Depends on Story 5.

---

### Edge Cases

- What happens when an onboarded resume cannot be parsed into the expected structure (e.g., the AI returns malformed data)? The onboarding step must surface a clear error rather than persist a half-parsed record.
- What happens when an existing resume (stored before this feature shipped) is loaded? Existing records have only raw text; they must continue to load without panic, and the user must have a clear path to re-parse.
- What happens when the skills section is empty after tailoring (all skills removed)? The structured form must tolerate empty sections and the final output must render sensibly.
- What happens when a required keyword and its alias both appear in the resume? The match is credited once; no double counting.
- What happens when an alias could match in two directions (e.g., JD asks for "Postgres" and resume has "PostgreSQL")? Either direction is recognized.
- What happens when the AI provides sections that don't match the agreed schema keys (e.g., uses "tech_stack" instead of "skills")? Onboarding rejects the payload with a schema-validation error listing the expected keys.
- What happens when the resume has a section that doesn't fit any schema key (e.g., "Military Service")? The AI must map it to the closest fitting key or note it in the raw text; out-of-schema keys are rejected.

## Requirements *(mandatory)*

### Functional Requirements — In Scope

- **FR-001**: The system MUST accept a structured sections representation of the resume at onboarding, alongside the raw text. In MCP mode the structured representation is provided by the orchestrating AI; in Headless (CLI) mode the structured representation is produced by an internal call to the existing LLM service over the raw text. TUI mode is out of scope (deprecation path).
- **FR-002**: The system MUST persist both the original raw text and the parsed sections for every resume.
- **FR-003**: The system MUST define a stable schema of section keys, with required keys (contact, experience) and optional keys (summary, skills, education, projects, certifications, awards, volunteer, publications) that onboarding validates against.
- **FR-004**: The tailoring steps (T1 and T2) MUST accept a unified edit envelope of the form `{ edits: [{ section, op, target?, value? }] }` where `op` is one of `add`, `remove`, `replace`. Both T1 and T2 accept this identical envelope shape; their only differences are scoring-tier routing hints returned in `next_action`.
- **FR-005**: The tailoring steps MUST apply edits directly to the corresponding section of the structured resume (no text-pattern heading detection, no regex substitution on raw text). Unknown targets MUST be rejected and reported in `edits_rejected` with a reason.
- **FR-005a**: Experience bullets MUST have stable, addressable IDs in the format `exp-<entry_index>-b<bullet_index>` (e.g., `exp-0-b2`). These IDs are returned in the sections payload by the scoring step and are used as `target` values in `replace`/`remove` edits to bullets. IDs for `add` operations on experience target an entry, not a bullet (format: `exp-<entry_index>`).
- **FR-006**: The scoring step MUST operate on a rendered text form of the current sections (reflecting any applied rewrites), not on the original raw text directly.
- **FR-007**: The keyword-match step within scoring MUST recognize a built-in set of industry-standard aliases (at minimum: Apache Spark↔PySpark, PostgreSQL↔Postgres, Kubernetes↔K8s, JavaScript↔JS, TypeScript↔TS) such that a resume containing one form matches a JD requiring the other.
- **FR-008**: The scoring step MUST NOT produce false positives from the alias system — in particular, existing word-boundary correctness for tokens like "C++", ".NET", "Go" must be preserved.
- **FR-009**: The scoring-step response MUST include the full structured sections map for the best-scoring resume, so the orchestrator can reason about structure without re-parsing.
- **FR-010**: The scoring-step response MUST distinguish "section not present" from "section present but empty" — the response structure must not silently omit populated sections.
- **FR-011**: The system MUST expose a `Renderer` interface with a default implementation that renders the structured sections to plain-text-equivalent output. The default Renderer MUST select section order dynamically based on the candidate's years-of-experience signal computed from `sections.experience`:
  - **Experienced tier (default):** `Contact → Summary → Experience → Skills → Education → Projects → Certifications → Awards → Volunteer → Publications`. Selected when total YoE ≥ 3.
  - **Entry-level tier:** `Contact → Summary → Education → Projects → Skills → Experience → Certifications → Awards → Volunteer → Publications`. Selected when `experience[]` is empty OR total YoE < 3.
  - **YoE computation:** sum of `experience[].{start,end}` spans in years, rounded down. Open-ended entries (`end == null` or `"present"`) count from `start` to today.
  - **Edge cases** (PhDs, career changers, returning workforce, long-tenure TAs/RAs): handled by the orchestrator via the `order` override (FR-011a), NOT by renderer heuristics on role titles.
  - Sections that are absent or empty are omitted from the output in both tiers.
- **FR-011a**: The sections payload MAY include an optional `order: [string]` field listing section keys in the desired output order. When present, the Renderer MUST use the provided order verbatim and MUST NOT apply tier selection. Any section key referenced in `order` but absent from the payload is skipped; any section present in the payload but missing from `order` is appended at the end in the default-tier order. This allows the orchestrator to handle edge cases (career changers, PhDs, bootcamp graduates) without baking candidate-type heuristics into the Renderer.
- **FR-011b**: The Renderer MUST emit canonical heading labels regardless of the labels the orchestrator used when providing the sections. Canonical labels: `Contact`, `Summary`, `Work Experience`, `Skills`, `Education`, `Projects`, `Certifications`, `Awards`, `Volunteer Experience`, `Publications`. This ensures ATS parsers that classify sections by recognized heading labels receive the expected labels even if the candidate's original résumé used variants (e.g., "Professional Experience", "Technical Stack", "Skills & Abilities").
- **FR-012**: The system MUST expose an `Extractor` interface with a default identity implementation that returns its text input unchanged.
- **FR-013**: The system MUST expose an MCP tool that returns the current resume as the Extractor would see it (identity output of the default Extractor today; real extraction later).
- **FR-014**: Resumes persisted before this feature shipped MUST load without error and MUST be re-parsed into the sections schema before any operation that depends on sections (scoring, tailoring, cover-letter generation) can run.
  - In Headless (CLI) mode, re-parse is performed automatically using the internal LLM service, transparent to the user beyond the added latency on the first operation after upgrade. After the first successful parse the sidecar is persisted; subsequent operations on the same record do not trigger additional LLM calls (one LLM call per record per upgrade, not per operation).
  - In MCP mode, operations requiring sections MUST return a structured error (distinct error code/type) indicating "sections missing — call add_resume with sections" so the orchestrator can re-onboard. The error MUST include the `raw` text so the orchestrator does not need to re-fetch it.
  - The system MUST NOT silently corrupt existing data, run in a degraded mode that mixes pre- and post-feature code paths, or produce scores against raw text for some records and sections-rendered text for others.
- **FR-015**: The final-output step MUST render the resume from the current structured sections, so any applied rewrites are reflected in what the user sees and submits.

### Functional Requirements — Deferred

The requirements below are specified now so future scope is clear, but are explicitly out of scope for this change.

- **FR-D01** (DEFERRED): The system SHOULD provide a real PDF rendering implementation of the `Renderer` interface, producing an output artifact suitable for submission to an ATS.
- **FR-D02** (DEFERRED): The system SHOULD provide a real text-extraction implementation of the `Extractor` interface (e.g., via `pdftotext`) that approximates what an ATS would extract from the rendered artifact.
- **FR-D03** (DEFERRED): The system SHOULD surface a "keyword-survival diff" comparing keywords present in the structured source to keywords recovered by extraction from the rendered artifact, so users can see layout-induced content loss.
- **FR-D04** (DEFERRED): The system SHOULD support multiple rendering templates (single-column ATS-safe as default; additional templates opt-in).
- **FR-D05** (DEFERRED): The system SHOULD support user-configurable alias overrides in addition to the built-in alias set.
- **FR-D06** (DEFERRED): The schema SHOULD be extensible to "tier 4" optional sections (languages, speaking engagements, open-source contributions, patents, interests, references) when real user demand appears.
- **FR-D07** (DEFERRED): The system SHOULD support ATS-specific extraction profiles (Workday, Greenhouse, Taleo, iCIMS) that approximate each platform's known extraction behavior.
- **FR-D08** (DEFERRED): The system SHOULD support per-section scoring weights (e.g., weighting keyword matches in "Experience" higher than matches in "Interests").

### Key Entities

- **Resume**: A user's résumé record. Has an immutable `raw` field (original text as submitted) and a mutable `sections` field (structured representation). Modified by tailoring steps; rendered for scoring and final output.
- **SectionMap**: The structured representation of a résumé. Contains a fixed set of named keys (`contact`, `experience` required; `summary`, `skills`, `education`, `projects`, `certifications`, `awards`, `volunteer`, `publications` optional). Each key holds either text, a list of structured entries, or a categorized map, depending on the section.
- **ExperienceEntry**: One job/role within the experience list. Has a stable ID (format `exp-<index>`), company, role, date range, optional location, and a list of bullet points. Each bullet has a stable ID (format `exp-<entry_index>-b<bullet_index>`) used as the `target` in tailoring edits.
- **EditEnvelope**: The input shape for tailoring operations. Contains a list of edits, each with `{ section, op, target?, value? }`. `op` is one of `add`, `remove`, `replace`. `target` references a skill string or a bullet ID depending on the section. `value` is the content for `add`/`replace` operations.
- **AliasSet**: A mapping from canonical keyword to its accepted surface forms (e.g., "Apache Spark" → ["PySpark"]). Consulted during keyword matching.
- **Renderer**: An abstraction that converts a `SectionMap` into output text or an output artifact. The default implementation renders to plain-text-equivalent output, selecting section order dynamically from the candidate's YoE signal (experienced tier vs entry-level tier) or honoring an optional orchestrator-provided `order` override, and always emits canonical heading labels regardless of input labels. Future implementations will render to PDF or DOCX.
- **Extractor**: An abstraction that converts a rendered artifact back into the text an ATS would see. Today the only implementation is identity (input equals output); tomorrow additional implementations will invoke real PDF text extractors.

## Success Criteria *(mandatory)*

### Measurable Outcomes — In Scope

- **SC-001**: Resumes with any reasonable skills-section heading (including "Skills & Abilities", "Technical Stack", "Core Competencies", and no heading at all) succeed at tailoring — the "zero substitutions silently succeeded" failure mode is eliminated.
- **SC-002**: For resumes containing industry-standard aliases (PySpark, Postgres, K8s, JS, TS), the scorer credits keyword matches at the same rate as it would for the canonical form, with no false positives from the alias system.
- **SC-003**: An AI orchestrator can inspect the structured contents of any section of a resume using only the response from the scoring step, without re-parsing raw text.
- **SC-004**: A maintainer can add a new rendering or extraction implementation (e.g., a PDF renderer) without modifying any scoring, tailoring, or MCP-envelope code.
- **SC-005**: Resumes persisted before this feature continue to load and function; at least 100% of existing records are either usable as-is or flagged for one-time re-parse, with zero silent data loss.
- **SC-006**: End-to-end regression: replaying the PlayStation Data Platform scenario (the run that motivated this feature) produces a score that credits the Apache Spark / PySpark match and a tailoring step that actually edits the skills section.

### Measurable Outcomes — Deferred

- **SC-D01** (DEFERRED): When a real PDF renderer and extractor are in place, the tool surfaces at least one concrete warning for multi-column or table-heavy layouts that drop keywords during extraction.
- **SC-D02** (DEFERRED): The scoring score reported to the user matches (within a small, documented margin) the score that an independent ATS-simulation tool would assign to the same rendered artifact.

## Assumptions

- In MCP mode, the orchestrating AI (Claude or equivalent) is responsible for parsing raw resume text into the structured sections schema at onboarding. The MCP server does not embed its own LLM for parsing.
- In Headless (CLI) mode, the existing internal LLM service is invoked at onboarding to produce the sections representation from the raw text. This adds one LLM call per `add_resume` / `onboard_user` operation in Headless mode; scoring and tailoring do not make additional LLM calls for parsing.
- TUI mode is on a deprecation path and is explicitly out of scope for this feature. TUI users continue to operate on the pre-existing (regex-based) tailoring path until TUI is removed.
- The structured sections schema is fixed for this feature. Adding or removing sections requires a schema-version bump.
- Users submit their final résumé as text or a simple ATS-safe layout. The deferred ATS-extraction feature addresses the case of richer layouts; for now, scoring trusts that the rendered text is what the ATS will see.
- The hardcoded alias set is curated and small (≤20 pairs) to minimize false-positive risk. Expansion requires either a config-driven override (deferred) or explicit change to the default set.
- Onboarding is an infrequent operation compared to scoring and tailoring; paying one LLM parse at onboarding is acceptable.
- Existing resumes (stored before this feature) are few enough that forcing a re-parse on next use is acceptable. Headless re-parses automatically; MCP surfaces a typed error prompting the orchestrator to call `add_resume` with sections. No background migration script is shipped.
- The Renderer's default plain-text output is sufficient for both scoring and final-output today. Future renderers will target PDF/DOCX.

## Dependencies

- The orchestrating AI (MCP client) must be capable of producing a valid structured sections payload at onboarding. This feature depends on the quality of that parse; if the AI returns malformed structure, onboarding fails loudly.
- The built-in alias dictionary must be maintained as the keyword landscape evolves (e.g., new frameworks, renamings). Maintenance is a documentation task, not a runtime dependency. **Owner:** project maintainer. **Trigger:** a user-reported false-negative (a well-known synonym not in the set) or a publicly documented framework rename (e.g., a tool previously known as X is now canonically called Y).
- The Renderer and Extractor interfaces are part of the feature's contract; future work (deferred features) plugs into them without modifying their signatures.

## Out of Scope

Explicitly out of scope for this feature:

- TUI mode — on a deprecation path; continues to use the existing regex-based tailoring until removal.

Deferred user stories tracked above:

- Producing actual PDF files from the structured resume.
- Running a real text-extraction library (e.g., `pdftotext`, Apache Tika).
- Warning users about layout-induced keyword loss.
- Supporting multiple output templates or ATS-specific extraction profiles.
- User-configurable alias dictionaries.
- Rare-section schema keys (languages, speaking, open-source, patents, interests, references).
- Per-section scoring weight tuning.
