# Feature Specification: Honest Scoring Loop — PDF Renderer, ATS Extractor, Keyword-Survival Diff

**Feature Branch**: `008-pdf-ats-extraction`
**Created**: 2026-04-25
**Status**: Draft
**Dependencies**: Spec 007 (section registry) — merged into `experimental` via PR #113

## Overview

Today `preview_ats_extraction` uses a stub renderer (returns plain text as-is) and an identity extractor (returns input unchanged). The score the tool reports is not the score an ATS will see. This feature closes that credibility gap by replacing both stubs with real implementations and adding a keyword-survival diff that surfaces which job-description keywords were lost during PDF rendering.

## Clarifications

### Session 2026-04-25

- Q: What shape should the `keyword_survival` field have in the `preview_ats_extraction` response? → A: Structured diff — `{"dropped": [...], "matched": [...], "total_jd_keywords": N}`
- Q: Should the PDF be held in memory or stored as a temp file during the render→extract pipeline? → A: In-memory only; writing PDF to a user-specified output file is explicitly future work.

## User Scenarios & Testing

### User Story 1 — Real PDF Generation (Priority: P1)

A job applicant asks go-apply to preview what an ATS will extract from their resume. The tool produces a real PDF from the resume data and returns the extracted plain text — the same text an ATS would index.

**Why this priority**: Without a real renderer, every score is fictional. This is the load-bearing story; the others depend on having real PDF bytes.

**Independent Test**: Call `preview_ats_extraction` with a populated resume; confirm the `constructed_text` field contains plain text extracted from an actual PDF (not the raw SectionMap dump).

**Acceptance Scenarios**:

1. **Given** a resume with Contact, Experience, and Skills sections, **When** `preview_ats_extraction` is called, **Then** the response contains plain text with the candidate's name, company names, and skill keywords present in readable form.
2. **Given** a resume with all 16 section types (including Tier 4), **When** `preview_ats_extraction` is called, **Then** all section headings and key content appear in the extracted text.
3. **Given** `pdftotext` is not installed, **When** `preview_ats_extraction` is called, **Then** the tool returns a clear error message and does not silently fall back to the identity extractor.

---

### User Story 2 — Keyword-Survival Diff (Priority: P2)

After seeing the extracted text, the applicant wants to know which job-description keywords were lost during PDF rendering — so they can fix their layout before submitting.

**Why this priority**: This is the primary user value of the honest loop. Without it, the user has raw extracted text but no actionable signal.

**Independent Test**: Load a job description with known keywords; ensure some keywords are present in the resume SectionMap and some are absent; confirm `preview_ats_extraction` returns a `keyword_survival` field where `dropped` lists exactly the SectionMap-absent keywords.

**Acceptance Scenarios**:

1. **Given** a JD with keywords ["machine learning", "kubernetes", "distributed systems"] and a resume with all three present, **When** rendered with an ATS-safe layout and extracted, **Then** `keyword_survival` shows zero dropped keywords.
2. **Given** a JD keyword that does not appear in any section of the resume SectionMap, **When** `preview_ats_extraction` is called, **Then** that keyword appears in `keyword_survival.dropped`.
3. **Given** a JD with zero keyword matches in the extracted text, **When** `preview_ats_extraction` is called, **Then** `keyword_survival` reflects 100% dropout (all JD keywords missing).

---

### User Story 3 — go-apply Doctor Preflight (Priority: P3)

A new user runs `go-apply` and receives a clear message telling them whether their system has the required `pdftotext` binary — before any scoring or extraction is attempted.

**Why this priority**: Fail-fast preflight prevents silent degradation. It is independently shippable and unblocks the error-handling story.

**Independent Test**: Run `go-apply doctor`; on a machine with `pdftotext` installed, output says it is present; on a machine without it, output gives a clear installation instruction.

**Acceptance Scenarios**:

1. **Given** `pdftotext` is installed and on PATH, **When** `go-apply doctor` is run, **Then** output confirms the dependency is satisfied.
2. **Given** `pdftotext` is absent from PATH, **When** `go-apply doctor` is run, **Then** output names the missing binary and provides an installation hint (e.g., "install poppler-utils").
3. **Given** `pdftotext` is absent, **When** `preview_ats_extraction` is called, **Then** the error message references `go-apply doctor` so the user knows how to diagnose the issue.

---

### Edge Cases

- What happens when the resume `SectionMap` is empty (only a contact name)? Renderer must produce a minimal valid PDF without crashing.
- What happens when JD has zero keyword matches in the extracted text? Survival diff must return all JD keywords as dropped, not panic or return nil.
- What happens when `pdftotext` is present but returns non-UTF-8 output? Extractor must handle encoding gracefully and surface an error.
- What happens when rendering a resume with only Tier 4 sections (no Experience, no Skills)? All populated sections must appear in extracted text.
- What happens when no structured SectionMap sidecar exists for the resume? `preview_ats_extraction` must return `no_sections_data` error — no silent fallback to raw resume text (FR-005b).
- What happens when `pdftotext` emits PII-containing stderr on failure? Error wrapping must sanitize stderr before logging (FR-010).

## Requirements

### Functional Requirements

- **FR-001**: The system MUST produce a PDF file from a `model.SectionMap` when rendering a resume.
- **FR-002**: The default PDF layout MUST be ATS-safe: single column, no tables, no text embedded in images, no decorative headers that ATS parsers skip.
- **FR-003**: The renderer MUST iterate sections via the section registry introduced in Spec 007, so new sections are included automatically without renderer code changes.
- **FR-004**: The system MUST extract plain text from PDF bytes using `pdftotext` (Poppler), returning the text an ATS would index.
- **FR-005**: The extractor MUST hard-fail with a descriptive error when `pdftotext` is not available — no silent fallback to the identity stub.
- **FR-005b**: `preview_ats_extraction` MUST hard-fail with error code `no_sections_data` when no structured SectionMap sidecar exists for the resume — no fallback to raw resume text. The error message MUST instruct the user to load a resume with structured sections.
- **FR-006**: `preview_ats_extraction` MUST return a `keyword_survival` field structured as `{"dropped": [...], "matched": [...], "total_jd_keywords": N}` — `dropped` lists JD keywords absent from extracted text; `matched` lists those present; `total_jd_keywords` is the count of all JD keywords evaluated.
- **FR-007**: `go-apply doctor` MUST check whether `pdftotext` is on PATH and emit a pass/fail result with an installation hint on failure.
- **FR-008**: Test golden files MUST capture extracted text content, not raw PDF bytes (PDF bytes are non-deterministic due to fonts and timestamps). The test suite MUST support a golden-regeneration mode activated by the `UPDATE_GOLDEN=1` environment variable; running tests with this flag set overwrites golden files rather than asserting against them.
- **FR-009**: The system MUST log when extraction is attempted, when `pdftotext` is invoked, and when keyword-survival diff is computed — with keyword counts in the log entry.
- **FR-010**: When `pdftotext` exits with a non-zero status, its stderr output MUST be captured, length-capped (max 256 bytes), and stripped of non-printable characters before being included in the returned error and any log entry. Raw stderr MUST NOT be forwarded as-is (it may contain resume PII).
- **FR-011**: The PDF renderer MUST validate all string inputs as valid UTF-8 before embedding them in the PDF. String content MUST be passed through the PDF library's documented text-escaping API — raw string concatenation into PDF output streams is prohibited. Invalid UTF-8 input MUST return an error rather than producing a corrupt PDF.

### Key Entities

- **PDF bytes**: The binary output of the renderer; passed to the extractor as `[]byte` (signature already fixed in Spec 007).
- **Extracted text**: The plain-text string an ATS would index; the ground truth for scoring and survival diff.
- **Keyword-survival diff**: A structured object `{"dropped": [...], "matched": [...], "total_jd_keywords": N}` comparing JD keywords against extracted text. `dropped` = keywords lost during rendering; `matched` = keywords that survived; `total_jd_keywords` = total evaluated.
- **Doctor report**: A structured pass/fail report of system dependencies required for honest scoring.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Rendering an ATS-safe resume layout produces zero keyword-survival warnings for keywords present in the resume source data.
- **SC-002**: When a resume is rendered whose source data does not contain a given JD keyword, that keyword appears in the `keyword_survival.dropped` list — confirming the survival diff correctly identifies absent keywords. (Multi-column layout dropout is deferred to FR-D04 parking lot; this criterion is testable with any ATS-safe layout by omitting keywords from the SectionMap.)
- **SC-003**: `preview_ats_extraction` response includes a `keyword_survival` field in all cases, structured as `{"dropped": [...], "matched": [...], "total_jd_keywords": N}` — `dropped` is empty when no keywords are lost; `total_jd_keywords` is zero when no JD is loaded.
- **SC-004**: `go-apply doctor` produces clear, human-readable output distinguishing the "all clear" and "missing dependency" cases.
- **SC-005**: All render and extract tests pass using extracted-text golden files; no test goldens on raw PDF bytes.
- **SC-006**: When `pdftotext` is unavailable, the error message emitted by the extractor mentions `go-apply doctor` as the diagnostic tool.

## Assumptions

- `pdftotext` (from the Poppler suite) is the chosen ATS extraction binary — it is the closest available proxy for real ATS text extraction behavior.
- PDF bytes are held in memory only — the rendered PDF is never written to disk during the render→extract pipeline. Writing the PDF to a user-specified output file is explicitly out of scope and deferred to future work.
- The PDF library choice (pure-Go library vs. headless Chrome) is a planning-phase decision; this spec is agnostic to that choice but requires that whichever is chosen hard-fails when unavailable rather than silently degrading.
- Spec 007's section registry is already merged into `experimental` and is the canonical iteration surface for the renderer.
- ATS-safe layout is defined as: single column, left-to-right reading order, no text-in-image, no floating elements, standard heading hierarchy.
- Multi-column layout (for demonstrating keyword dropout) is out of scope for the default renderer but may be added as a second template later (FR-D04, parking lot).
- The JD keyword set used for the survival diff is the same set already computed by the scorer (no separate extraction pass required).
