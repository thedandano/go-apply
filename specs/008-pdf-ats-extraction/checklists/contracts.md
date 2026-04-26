# Interface Contract Checklist: Honest Scoring Loop

**Purpose**: Validate completeness, consistency, and plan adherence of interface contracts and service boundaries
**Created**: 2026-04-25
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md)

## Requirement Completeness — Interface Contracts

- [x] CHK001 - Is the `port.PDFRenderer` interface contract specified for all input states: nil, empty, partial, and fully-populated `SectionMap`? [Completeness, contracts/pdf-renderer.md]
- [x] CHK002 - Is the render ordering requirement (section registry order must match plain-text renderer order) specified in the `port.PDFRenderer` contract? [Completeness, contracts/pdf-renderer.md]
- [x] CHK003 - Is the `port.Extractor` contract explicit that no fallback to identity behavior is permitted under any condition? [Completeness, Spec §FR-005]
- [x] CHK004 - Is the subprocess timeout value for `pdftotext` invocation specified as a requirement (not just a planning detail)? [Completeness, contracts/extractor.md]
- [x] CHK005 - Are `survival.Service.Diff` invariants (dropped + matched = total_jd_keywords, no-nil slices) specified as testable contract assertions rather than implementation notes? [Completeness, contracts/survival-service.md]
- [x] CHK006 - Is the `go-apply doctor` exit code contract (0 = all clear, 1 = any missing) explicitly specified? [Completeness, contracts/doctor-command.md]

## Requirement Clarity — Ambiguities in Contract Definitions

- [x] CHK007 - Is "lowercase normalized" in `data-model.md` (KeywordSurvival entity) consistent with "case-insensitive regexp matching" in `contracts/survival-service.md`? Are these the same requirement stated differently, or two separate constraints? [Ambiguity, data-model.md + contracts/survival-service.md]
- [x] CHK008 - Is "ATS-safe layout" defined measurably in the `port.PDFRenderer` contract, or only in the spec's Assumptions section? [Clarity, Spec §Assumptions, contracts/pdf-renderer.md]
- [x] CHK009 - Is the `sections_used` field in `previewData` still meaningful after silent fallbacks are removed? Are its semantics updated to reflect that `false` is now an error condition rather than a fallback path? [Clarity, data-model.md, Spec §FR-005]
- [x] CHK010 - Does the spec explicitly state that `keyword_survival.total_jd_keywords` will never be zero in practice (since `preview_ats_extraction` requires `stateScored`, which requires a JD)? Or is the zero-case a live requirement? [Clarity, Spec §Clarifications, Spec §US1 Acceptance Scenario 3]

## Requirement Consistency — Cross-Document Alignment

- [x] CHK011 - Is the `KeywordSurvival` struct definition consistent across spec, data-model.md, contracts/survival-service.md, and the clarification session (`{"dropped": [...], "matched": [...], "total_jd_keywords": N}`)? [Consistency]
- [x] CHK012 - Does the `port.PDFRenderer` contract's in-memory constraint (no temp files) align with the spec's Assumptions section and the clarification answer? Are both expressed with identical scope? [Consistency, Spec §Clarifications, contracts/pdf-renderer.md]
- [x] CHK013 - Are the two silent fallback removal sites (session_tools.go:714 and session_tools.go:719) both explicitly named as in-scope changes in the plan, and are their replacement behaviors (hard errors) specified consistently with FR-005? [Consistency, plan.md §Constitution Check, Spec §FR-005]
- [x] CHK014 - Is the error code naming convention for new error codes (`pdftotext_unavailable`, `render_failed`, `no_sections_data`) consistent with existing error codes in the MCP response envelope? [Consistency, contracts/extractor.md, contracts/doctor-command.md]

## Acceptance Criteria Quality — Measurability of Contract Assertions

- [x] CHK015 - Can the "rendered PDF, when extracted by pdftotext, produces text matching the plain-text renderer output for the same SectionMap" requirement (contracts/pdf-renderer.md) be objectively verified? Is there a golden-file strategy defined? [Measurability, Spec §SC-005, research.md §Golden Test Strategy]
- [x] CHK016 - Is SC-002 ("rendering a layout that causes keyword dropout surfaces those dropped keywords") measurable without a multi-column layout renderer? The default renderer is ATS-safe (single column) — is there a defined test fixture for demonstrating dropout? [Measurability, Spec §SC-002]
- [x] CHK017 - Is SC-004 ("go-apply doctor produces clear, human-readable output") measurable? Is the exact output format specified enough to write a deterministic assertion against it? [Measurability, Spec §SC-004, contracts/doctor-command.md]

## Scenario Coverage — Plan Adherence

- [x] CHK018 - Is US2 (keyword-survival diff) dependency on US1 (PDF renderer) explicitly declared in the spec and plan? The spec says US1 is "load-bearing" — is this framed as a hard prerequisite, not just a priority ordering? [Coverage, Spec §US1, plan.md]
- [x] CHK019 - Is the `go-pdf/fpdf` dependency addition to `go.mod` addressed as an in-scope change in the plan's source code layout? [Coverage, Gap, plan.md §Source Code Changes]
- [x] CHK020 - Is the `UPDATE_GOLDEN=1` test regeneration mechanism specified as a requirement or only as a research note? If it is a requirement, does FR-008 (extracted-text golden files) cover it explicitly? [Coverage, Spec §FR-008, research.md §Golden Test Strategy]
- [x] CHK021 - Are the changes to `HandlePreviewATSExtractionWithConfig` (wiring PDFRenderer, removing fallbacks, adding KeywordSurvival) explicitly listed in the plan's source code changes? [Coverage, plan.md §Source Code Changes]
- [x] CHK022 - Is the `survival.Service` keyword-set derivation formula (ReqMatched ∪ ReqUnmatched ∪ PrefMatched ∪ PrefUnmatched) specified as a requirement in the spec, or only in the contracts document? [Coverage, Spec §FR-006, contracts/survival-service.md]

## Edge Case Coverage — Missing Boundary Definitions

- [x] CHK023 - Is the behavior specified when `survival.Diff` receives duplicate keywords (same keyword in both req and pref lists)? Should duplicates be deduplicated before matching? [Edge Case, Gap, contracts/survival-service.md]
- [x] CHK024 - Is there a requirement for what `pdftotext` does with PDF bytes that are valid but produce empty extracted text (e.g., image-only PDF)? Should this be an error or return empty string? [Edge Case, Gap, contracts/extractor.md]
- [x] CHK025 - Is the behavior specified if `go-apply doctor` is run as a non-interactive subprocess (e.g., in CI)? Does the output format assumption (plain text with [OK]/[MISSING] prefixes) hold for machine parsing? [Edge Case, contracts/doctor-command.md]

## Dependencies & Assumptions

- [x] CHK026 - Is the assumption that "the scorer's keyword set is always available in sess.ScoreResult.Keywords before preview_ats_extraction is called" validated by the session state machine? Is `stateScored` sufficient to guarantee a non-empty KeywordResult? [Assumption, Spec §Assumptions, contracts/survival-service.md]
- [x] CHK027 - Is the `go-pdf/fpdf` library's ATS compatibility (no invisible text layers, no embedded scripts) an explicit assumption or a verified property? [Assumption, Spec §Assumptions, research.md §Decision 1]
