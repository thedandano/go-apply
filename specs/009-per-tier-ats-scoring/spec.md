# Feature Specification: Per-Tier Real-World ATS Scoring

**Feature Branch**: `009-per-tier-ats-scoring`
**Created**: 2026-04-25
**Status**: Ready for Plan

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Latin-1 Safe Resume Content (Priority: P1)

A user uploads a resume that contains common typographic characters — em-dashes, smart
quotes, bullet points — or tailors a resume where the LLM inserts such characters during
skill rewrites. The system transparently normalises these characters before generating any
PDF, so the tailoring and ATS scoring pipeline never fails due to a character encoding
error.

**Why this priority**: Without this, any resume with a non-ASCII punctuation character
blocks the entire pipeline. Tier-1 LLM rewrites regularly insert smart quotes. This is a
hard prerequisite for every other story in this spec.

**Independent Test**: Upload or tailor a resume containing em-dashes and smart quotes.
Verify the PDF is generated successfully and the characters appear as their ASCII
equivalents in the extracted text.

**Acceptance Scenarios**:

1. **Given** a resume section containing an em-dash (U+2014), **When** the PDF is
   rendered, **Then** the PDF is produced without error and the em-dash appears as a
   hyphen in extracted text.
2. **Given** a skill bullet rewritten by the LLM that contains smart quotes (U+2018,
   U+2019), **When** Tier-1 tailoring completes, **Then** the tailoring step succeeds and
   the quotes are normalised to straight ASCII quotes.
3. **Given** a `Contact.Name` field containing a Latin-extended character (e.g., é, ü),
   **When** the PDF is rendered, **Then** the character is substituted with its ASCII base
   letter without crashing (é→e, ü→u).
4. **Given** a character with no ASCII equivalent (e.g., U+2603 snowman, U+1F600 emoji),
   **When** the PDF is rendered, **Then** the system substitutes `?`, logs a warn-level
   message, and does not fail.
5. **Given** a resume where the Contact name, Skills flat text, and Experience bullets all
   contain em-dashes simultaneously, **When** the PDF is rendered, **Then**
   transliteration applies uniformly to all three fields — not just Summary and Experience.

---

### User Story 2 — Consistent ATS Score at Every Tier (Priority: P1)

A user runs the full tailoring cascade (T0 → T1 → T2). At each stage they see a score
that reflects exactly what an ATS system would compute after extracting text from the
submitted PDF — not a score from an internal text render that diverges from the actual
PDF output. The T0, T1, and T2 scores are directly comparable because they all use the
same measurement method.

**Why this priority**: The entire value of the score-progression view is lost if T0 uses
one scoring method and T1/T2 use another. Users need confidence that an improved score
means their resume will perform better, not that a rendering difference produced an
artefact.

**Independent Test**: Run `submit_keywords` → `submit_tailor_t1` on a resume. Verify
that the `previous_score` in the T1 response and the `new_score` are both derived from
the PDF→extract path, producing comparable numbers.

**Acceptance Scenarios**:

1. **Given** a session that has just completed `submit_keywords`, **When** `submit_tailor_t1`
   is called, **Then** the `previous_score` field reflects a PDF-extracted baseline (not
   a plain-text score) and the `new_score` reflects the PDF-extracted score after T1 edits.
2. **Given** a session at T1 applied state, **When** `submit_tailor_t2` is called,
   **Then** the `previous_score` is the T1 PDF-extracted score and `new_score` is the T2
   PDF-extracted score.
3. **Given** that PDF extraction returns an error or empty text (e.g., a PDF whose `%%EOF`
   marker is truncated), **When** any tier attempts to score, **Then** the tier returns a
   hard error to the caller — it does not silently degrade to plain-text scoring.
4. **Given** `submit_keywords` is called with N resumes where one PDF is corrupted,
   **When** concurrent extraction runs, **Then** the entire call returns a hard error
   identifying the failing resume label; no partial scores are emitted.
5. **Given** a session after this spec ships, **When** `preview_ats_extraction` is called
   against the same PDF that was scored at T0, **Then** keyword survival results are
   consistent with those produced before the `pdftotext` → `ledongthuc/pdf` swap (same
   matched/dropped sets for the same resume content).
6. **Given** a successful T1 call, **When** the response JSON is inspected, **Then** the
   `scoring_method` field is present and equals `"pdf_extracted"`.

---

### User Story 3 — Threshold Calibrated to Real PDF-Extracted Scores (Priority: P2)

The routing decision after Tier-1 (`tailor_t2` vs `cover_letter`) is based on a score
threshold that was calibrated against plain-text scores. After the scoring method changes
to PDF-extracted, the user's real resume is used to measure the systematic gap between
old and new scores. The threshold is updated so that the routing decision still reflects
the intended quality bar, not a rendering artefact.

**Why this priority**: Without recalibration, the routing logic may incorrectly skip T2
(directing the user to cover letter) or incorrectly require T2 (blocking cover letter
generation) simply because PDF-extracted scores are systematically offset from
plain-text scores.

**Independent Test**: Run the user's real resume through both the old plain-text scorer
and the new PDF-extracted scorer against the same set of job descriptions. Record the
delta. Verify the updated threshold produces the same routing decisions that the old
threshold produced on the same inputs.

**Acceptance Scenarios**:

1. **Given** the user's real resume scored against three or more representative JDs,
   **When** both plain-text and PDF-extracted scores are compared, **Then** the mean
   delta is measured and documented.
2. **Given** the measured delta, **When** the threshold is updated, **Then** routing
   decisions (`tailor_t2` vs `cover_letter`) match the intent of the original calibration
   on the same resume+JD pairs.
3. **Given** a resume that previously routed to `cover_letter` under the old threshold,
   **When** scored with PDF-extraction and the recalibrated threshold, **Then** the
   routing decision is unchanged.

---

### Edge Cases

- What happens if `ledongthuc/pdf` returns an error or empty text (e.g., corrupted PDF)?
  → Hard error surfaced to caller; tailoring step fails explicitly. No silent fallback.
- What happens if the PDF renderer produces a zero-length PDF? → Hard error surfaced to
  caller; tailoring step fails explicitly.
- What happens if the resume has no structured sections sidecar? → Existing error path
  (`sections_missing`) returned unchanged — Latin-1 fix and PDF scoring don't affect
  this path.
- What happens when a character cannot be transliterated to any ASCII equivalent? →
  Substitute with a safe placeholder (e.g., `?`) and log at warn level.

## Requirements *(mandatory)*

### Functional Requirements

**Latin-1 fix (prerequisite)**

- **FR-001**: The PDF renderer MUST accept any valid UTF-8 input by transliterating
  characters outside Latin-1 to their closest ASCII equivalent before rendering, rather
  than rejecting them. Transliteration MUST apply to all string fields in `SectionMap`
  (Contact, Summary, Experience, Education, Skills, Projects, and all remaining sections).
- **FR-002**: The transliteration MUST cover at minimum: em-dash (U+2014) → `-`, en-dash
  (U+2013) → `-`, bullet (U+2022) → `-`, smart single/double quotes (U+2018/U+2019/U+201C/U+201D)
  → straight quotes, ellipsis (U+2026) → `...`, non-breaking space (U+00A0) → regular space,
  hyphen variants (U+2010, U+2011) → `-`, and common Latin-extended accented letters → their
  ASCII base letter (e.g., é → e, ü → u, ñ → n). Currency symbols, mathematical operators,
  and other non-typographic Unicode are out of scope and fall through to the safe placeholder
  path (FR-003).
- **FR-003**: Characters with no reasonable ASCII equivalent (e.g., U+2603 snowman,
  U+1F600 emoji, currency symbols, mathematical operators) MUST be substituted with `?`
  and logged at `warn` level including only the codepoint and section field name — the
  surrounding resume text MUST NOT be included in the log message. They MUST NOT cause a
  hard failure.
- **FR-012**: The PDF renderer MUST NOT contain string literals with characters outside
  Latin-1. Any format separator strings in `pdfrender.go` that use `—` (U+2014 em-dash,
  e.g., `" — "` separating title from issuer/venue/event) MUST be replaced with `" - "`
  (ASCII hyphen). Transliteration operates on `*model.SectionMap` user input only; it does
  not process renderer-generated strings. This requirement closes the latent rendering
  failure for Certifications, Publications, Speaking, and Open Source sections.

**Consistent per-tier PDF scoring**

- **FR-004**: The T0 baseline score stored during `submit_keywords` MUST be derived from
  PDF→extract for ALL resumes in the call, using the same pipeline as T1 and T2. Renders
  MUST run concurrently using `errgroup.WithContext` (one goroutine per resume) so total
  latency is bounded by the slowest single resume and in-flight goroutines are cancelled
  when any one fails. If extraction fails for any resume, the call MUST return a hard
  error — no silent fallback to plain-text scoring.
- **FR-005**: After T1 edits are applied, the score returned in the `submit_tailor_t1`
  response MUST be derived from the PDF→extract pipeline (render PDF from new sections →
  extract text → score).
- **FR-006**: After T2 edits are applied, the score returned in the `submit_tailor_t2`
  response MUST be derived from the PDF→extract pipeline.
- **FR-007**: Each tier response MUST include a `ScoringMethod string` field populated
  from the package-level constant `ScoringMethodPDFExtracted = "pdf_extracted"` — the
  only supported method. Using a named constant (not a string literal) ensures all three
  tiers stay in sync and prevents future drift. This field is diagnostic-only: the MCP
  host MAY surface it but MUST NOT branch on its value in this spec.
- **FR-009**: T0, T1, and T2 MUST all use a common internal helper for the
  PDF→extract→score sequence, with the signature:
  `scoreSectionsPDF(ctx, sections, label, sessionID, jd, cfg, deps) (model.ScoreResult, error)`.
  The `sessionID` parameter MUST be included in every structured log entry emitted by the
  helper (as `"session_id", sessionID`) so a complete scoring request can be reconstructed
  from logs alone. Callers set `ScoringMethod = ScoringMethodPDFExtracted` on the response
  after the call — the helper does not return it. T1 and T2 pass already-applied sections;
  T0 passes the original sections without an edit step. All PDF scoring logic lives in this
  one function and cannot drift between tiers.

- **FR-013**: The `ledongthuc/pdf` extractor output MUST be equivalent to the prior
  `pdftotext` output for the purpose of keyword-survival scoring. Equivalence is defined
  as: identical matched/dropped keyword sets across all 12 JDs in the calibration suite.
  Whitespace and line-break differences are acceptable as long as keyword tokenisation
  produces identical sets. This is verified by US2 §5.
- **FR-014**: The headless CLI pipeline (`internal/service/pipeline`) MUST NOT depend on
  `port.Extractor` or `extract.Service`. A grep or import-graph check in the Polish phase
  (T014) MUST confirm the package contains no reference to the extractor port, preventing
  accidental adoption of PDF extraction in the headless flow.

**Threshold recalibration — COMPLETED**

- **FR-010/FR-011** *(completed pre-implementation)*: Calibration ran against 12 real
  cached JDs using the user's real resume. Mean delta: `+0.87 pts` (PDF-extracted scores
  are slightly higher than plain-text). The `NextActionAfterT1` threshold of `70.0`
  requires no change — the delta is below 1 pt and does not shift any routing decision.

### Key Entities

- **ScoringMethod**: A string constant (`"pdf_extracted"`) attached to every tier
  response, confirming the extraction path used. No other value is valid in this spec.
- **ThresholdCalibration**: An informal measurement artefact (not a data model) — the
  delta between plain-text and PDF-extracted scores across a test suite, used to set the
  routing threshold. *Completed: delta +0.87 pts, threshold 70.0 unchanged.*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every character listed in FR-002 (em-dash, en-dash, smart quotes ×4, ellipsis,
  bullet, non-breaking space, hyphen variants U+2010/U+2011, and every codepoint in the
  U+00C0–U+00FF accented-Latin block per the data-model mapping table) plus at least two
  unmappable codepoints (U+2603, U+1F600) pass through `RenderPDF` in a table-driven test
  without error, producing the expected ASCII substitution in the extracted text.
- **SC-002**: The T0, T1, and T2 scores reported in a single session are all produced by
  the same scoring method (verifiable via `scoring_method` field in each response).
- **SC-003**: The `scoring_method` field reads `"pdf_extracted"` for all three tiers in
  every successful session.
- **SC-004**: Routing decisions (`tailor_t2` vs `cover_letter`) for the user's real resume
  against the 12 JDs in `~/.local/share/go-apply/jd_cache/` used in the 2026-04-26
  calibration run (see research.md Decision 6) match the intent of the pre-change routing
  in 100% of tested cases.
- **SC-005**: *(Design intent, not a CI-measurable criterion)* The shared `scoreSectionsPDF`
  helper is the only site that calls `deps.Extractor.Extract`. Per-tier handlers do not
  call the extractor directly.

## Assumptions

- `ledongthuc/pdf` (pure-Go library, already in `go.mod`) is the extraction backend; no
  OS binary or subprocess is required.
- Extraction failure (error or empty text) is treated as a hard error — no plain-text
  fallback. The caller surfaces the error explicitly.
- The user's real resume is available in the go-apply data directory and can be used for
  threshold calibration runs locally without additional setup.
- Threshold calibration is complete: mean delta +0.87 pts (PDF > plain-text), threshold
  70.0 unchanged across all 12 tested JDs.
- The existing `port.PDFRenderer`, `port.Extractor`, and `port.SurvivalDiffer` interfaces
  remain unchanged; only wiring and invocation sites change.
- The `extract.Service` adapter swap (`pdftotext` → `ledongthuc/pdf`) is **system-wide**
  — it affects every caller of `port.Extractor`, including `preview_ats_extraction`. The
  headless CLI pipeline (`pipeline.ScoreResumes`) does not use the extractor and is
  unaffected. All `port.Extractor` consumer sites MUST be identified via grep before the
  PR lands.
- The headless CLI scoring path (`pipeline.ScoreResumes` → plain-text render) is
  intentionally unchanged by this spec. MCP scoring uses PDF; headless uses plain text.
  This is a deliberate boundary, not an oversight.
- `preview_ats_extraction` is retained without functional changes: (a) the tool's input
  arguments and JSON schema are unchanged, (b) the response shape (`model.KeywordSurvival`
  plus `ats_extracted_text`) is byte-identical in structure, (c) error codes and messages
  emitted by the tool are unchanged, and (d) the keyword survival match/drop sets MUST be
  equivalent for the same input PDF before and after the `pdftotext` → `ledongthuc/pdf`
  swap (verified by US2 §5).
- `transliterateLatin1` returns a **deep copy** of sections — the original `SectionMap`
  is never mutated. Callers MUST NOT persist the sanitised copy as the resume sidecar.
- Cross-session score comparisons require matching `scoring_method`. The ~+0.87 pt shift
  from plain-text to PDF-extracted means pre-spec and post-spec scores are not directly
  comparable; the `scoring_method` field is the disambiguator.
- `scoring_method` is diagnostic-only in this spec. The MCP host MAY display it but MUST
  NOT branch on its value. Host-prompt guidance is deferred to `010-ats-scoring-surface`.

## Deferred Work

The following items were identified during design but are explicitly out of scope for this
spec. They depend on this work completing first. All five items ship together in the next
spec (`010-ats-scoring-surface`).

**`010-ats-scoring-surface` — one bundled follow-on spec**

- **Schema additions**: `ats_keyword_survival` (`model.KeywordSurvival`) MUST be included
  in every T1/T2 response unconditionally — it is small and is the primary ATS signal the
  MCP host needs without calling a separate tool. The full `ats_extracted_text` string is
  large and MUST be gated behind an explicit request flag; it is omitted by default.
- **`preview_ats_extraction` narrowed (not removed)**: The tool is retained as a
  pre-tailor baseline diagnostic — useful after `submit_keywords` and before any edits are
  applied. Its documented purpose in `prompt.go` is updated in three places (tool table,
  T1/T2 Returns docs, ATS preview section) to reflect that post-T1/T2 use is redundant.
- **Session caching (in-memory only)**: PDF-extracted text is stored in the session struct
  after each tier completes. The cache is in-memory only — no disk persistence. It is
  invalidated when sections change (i.e. when a new tier applies edits). This avoids
  re-running the PDF→extract pipeline when `preview_ats_extraction` is called in the same
  session against an already-scored tier.
- **`next_action_rationale` (conditional)**: The field is emitted in the T1 response only
  when an extraction artefact — not actual resume quality — influenced the routing
  decision (e.g., ledongthuc/pdf dropped a term that caused the score to fall below
  threshold). It is absent when routing was unaffected.

- **`011-bm25-keyword-scoring` (maybe)**: Replace `keyword_match` internals with full Okapi BM25 using a pre-computed IDF table built from a public job postings dataset (e.g. Kaggle). Current spike proved TF-saturation alone doesn't outperform binary on small corpora — real IDF signal requires a large corpus. No schema or API changes needed. Defer until there is a clear user-facing pain point that binary scoring can't address.

## Clarifications

### Session 2026-04-25

- Q: Should T0 reuse the shared PDF→extract→score helper or have its own path? → A: T0 reuses the shared helper (skipping the edit step) — all PDF scoring logic in one place.
- Q: How broad should the character transliteration mapping be? → A: Common typographic characters (em-dash, smart quotes, ellipsis) and accented Latin letters (é→e, ü→u). Currency symbols and other non-typographic Unicode fall through to the safe placeholder (`?`) path.
- Q: What timeout should apply to the pdftotext extraction step? → ~~A: 30 seconds per resume at every tier, uniform. Timeout counts as unavailability and triggers plain-text fallback with a warning.~~ *Superseded by Session 2026-04-26 — pdftotext removed; no subprocess, no timeout, no fallback.*
- Q: During `submit_keywords`, should PDF rendering run for all resumes or only the best-scored one? → A: All resumes — fidelity requires every candidate measured on the same pipeline; renders run concurrently (errgroup.WithContext) so latency is bounded by the slowest resume. ~~When pdftotext is unavailable, ALL resumes fall back uniformly.~~ *Fallback clause superseded — extraction failure is now a hard error (Session 2026-04-26).*
- Q: Should the 5 deferred items ship as one spec or be split? → A: One bundled spec (all 5 items together as `010-ats-scoring-surface`).
- Q: Should `ats_keyword_survival` in T1/T2 be always-present or opt-in? → A: Always-present; raw `ats_extracted_text` is gated behind a request flag.
- Q: Should `preview_ats_extraction` be removed, kept, or narrowed? → A: Narrowed — retained as pre-tailor baseline diagnostic only; not removed.
- Q: Session cache: in-memory only or persisted to disk? → A: In-memory only; invalidated when sections change.
- Q: Should `next_action_rationale` be always present or conditional? → A: Conditional — emitted only when an extraction artefact influenced routing.

### Session 2026-04-26

- Q: Which PDF extraction library? → A: `ledongthuc/pdf` (pure-Go, already in go.mod). No OS subprocess, no pdftotext dependency anywhere in the spec.
- Q: Is a plain-text fallback needed when extraction fails? → A: No. The constitution requires fail-fast and no silent fallbacks. Extraction failure (error or empty text) surfaces as a hard error to the caller. FR-008 and FR-008a removed.
- Q: Is FR-010/FR-011 (threshold calibration) still open work? → A: No. Calibration completed: mean delta +0.87 pts across 12 real JDs, threshold 70.0 unchanged. Converted to a completed note.
- Q: What does ScoringMethod look like as a concrete field? → A: `ScoringMethod string` on the tier response struct, value `"pdf_extracted"` only. Dual-value enum removed — no fallback, no second value needed.
