# Regression Checklist: Per-Tier Real-World ATS Scoring

**Purpose**: Validate that requirements adequately protect existing behavior during the extractor swap (pdftotext → ledongthuc/pdf), Latin-1 transliteration, and per-tier scoring changes
**Created**: 2026-04-26
**Resolved**: 2026-04-26 (council review)
**Feature**: [spec.md](../spec.md) · [plan.md](../plan.md) · [data-model.md](../data-model.md)
**Scope**: Extraction adapter, Latin-1 transliteration, T0/T1/T2 scoring path, preview_ats_extraction, headless CLI path

---

## Extraction Adapter Swap (pdftotext → ledongthuc/pdf)

- [x] CHK001 — Are requirements defined for what counts as "equivalent" extraction output between ledongthuc/pdf and pdftotext (whitespace, line breaks, character fidelity)? [Clarity, Gap] → **Fixed**: FR-013 added — equivalence defined as identical matched/dropped keyword sets across the 12-JD calibration suite; whitespace/line-break differences acceptable.
- [x] CHK002 — Does the spec define what "empty text" output means as a failure condition for ledongthuc/pdf? [Clarity, Spec §FR-004] → **PASS**: FR-004 + Edge Cases + data-model.md §Empty-text detection all define `len(text)==0` as a hard error.
- [x] CHK003 — Are error handling requirements for corrupted PDFs specified consistently for the new extractor? [Consistency, Spec §FR-004, Edge Cases] → **PASS**: Edge Cases, FR-004, and Assumptions all agree: hard error, no fallback.
- [x] CHK004 — Is the `port.Extractor` interface contract sufficient to guarantee callers won't need changes when the implementation swaps? [Completeness, Spec §Assumptions, data-model.md] → **PASS**: Assumptions + data-model.md confirm interface is unchanged; callers require no changes.

## Latin-1 Transliteration

- [x] CHK005 — Are requirements complete for all character ranges that commonly appear in real resumes (em-dash, smart quotes, accented letters, bullets)? [Completeness, Spec §FR-002] → **PASS**: FR-002 enumerates em-dash, en-dash, bullet, smart quotes ×4, ellipsis, non-breaking space, hyphen variants, accented Latin U+00C0–U+00FF.
- [x] CHK006 — Does the spec require transliteration to produce a non-destructive copy of sections rather than mutating in place? [Clarity, Gap] → **PASS**: Assumptions explicitly states "`transliterateLatin1` returns a **deep copy** of sections — the original `SectionMap` is never mutated."
- [x] CHK007 — Are requirements defined for which string fields receive transliteration (all fields vs. a specific subset)? [Completeness, Spec §FR-001] → **PASS**: FR-001 says "all string fields in `SectionMap` (Contact, Summary, Experience, Education, Skills, Projects, and all remaining sections)."
- [x] CHK008 — Is the logging requirement for substituted characters specific enough to be deterministic (log level, what to include)? [Clarity, Spec §FR-003] → **PASS**: FR-003 specifies warn level, codepoint + field name only, and explicitly excludes surrounding resume text.
- [x] CHK009 — Is the behavior for characters outside Latin-1 with no ASCII equivalent (`?` substitution) specified precisely enough that two developers would implement it identically? [Clarity, Spec §FR-003] → **PASS**: FR-003 + FR-002's exclusion list + Acceptance Scenario 4 (U+2603/U+1F600 → `?`) make implementation deterministic.

## Per-Tier Scoring (T0 / T1 / T2)

- [x] CHK010 — Are requirements defined for what happens at T0 when a resume has no sections sidecar (sections file missing)? [Coverage, Spec §Edge Cases] → **PASS**: Edge Cases: "Existing error path (`sections_missing`) returned unchanged."
- [x] CHK011 — Is the hard-error requirement at T0 (fail whole call if any resume fails extraction) consistent with T1/T2 which also fail hard on extraction error? [Consistency, Spec §FR-004, FR-005, FR-006] → **PASS**: FR-004/005/006 all route through `scoreSectionsPDF` (FR-009); US2 §3 confirms "any tier" returns a hard error.
- [x] CHK012 — Does the spec require that `previous_score` in T1/T2 responses reflects a PDF-extracted score (not a plain-text score from before this change)? [Completeness, Spec §User Story 2 §1–2] → **PASS**: US2 Acceptance Scenarios 1–2 explicitly require PDF-extracted `previous_score` and `new_score`.
- [x] CHK013 — Is the `ScoringMethod` constant value `"pdf_extracted"` specified consistently across T0, T1, and T2 response definitions? [Consistency, Spec §FR-007, SC-002, data-model.md] → **PASS**: FR-007 defines the constant once; data-model.md shows all three tiers reference it; SC-003 asserts the value.
- [x] CHK014 — Are observability requirements for `scoreSectionsPDF` (what to log at render/extract/score stages) unambiguous enough to be verified in a PR review? [Clarity, Spec §Constitution V, data-model.md] → **PASS**: data-model.md enumerates four exact log entries with all field names; FR-009 mandates `session_id` in every entry.
- [x] CHK015 — Does SC-005 ("changing extraction requires editing exactly one function") remain accurate given T0 has concurrent goroutines that each call the helper? [Consistency, Spec §SC-005] → **PASS**: Concurrent goroutines call the same single helper function — "exactly one site" remains true regardless of call count.

## preview_ats_extraction Regression

- [x] CHK016 — Does the spec explicitly state that `preview_ats_extraction` behavior is unchanged after the extractor swap, or is this only implied? [Completeness, Spec §Assumptions] → **PASS**: Assumptions explicitly states retention without functional changes.
- [x] CHK017 — Is the meaning of "retained without functional changes" for `preview_ats_extraction` defined precisely (same input contract, same output format, same error codes)? [Clarity, Spec §Assumptions] → **Fixed**: Assumption now enumerates (a) input/schema unchanged, (b) `model.KeywordSurvival` + `ats_extracted_text` structure unchanged, (c) error codes unchanged, (d) match/drop sets equivalent.
- [x] CHK018 — Are there requirements confirming that `preview_ats_extraction` produces comparable results before and after the ledongthuc/pdf swap (not just that it compiles)? [Gap] → **PASS**: US2 Acceptance Scenario 5 is a concrete comparable-results assertion (same matched/dropped sets for same input PDF).

## Headless CLI Path

- [x] CHK019 — Does the spec explicitly state that the headless CLI scoring path (`ScoreResumes` → plain-text render) is intentionally unchanged by this feature? [Completeness, Gap] → **PASS**: Assumptions: "intentionally unchanged by this spec ... deliberate boundary, not an oversight."
- [x] CHK020 — Are requirements defined to prevent the new `extract.Service` implementation from being accidentally used in the headless CLI pipeline? [Coverage, Gap] → **Fixed**: FR-014 added — `internal/service/pipeline` MUST NOT depend on `port.Extractor`; verified by T014 grep/import-graph check.
- [x] CHK021 — Is the boundary between MCP-mode scoring (PDF) and headless-mode scoring (plain-text) documented as an explicit design decision in the spec or plan? [Clarity, Gap] → **PASS**: Assumptions in spec.md: "MCP scoring uses PDF; headless uses plain text. This is a deliberate boundary, not an oversight."

## Acceptance Criteria Quality

- [x] CHK022 — Is SC-001 (100% PDF rendering success for transliterated resumes) measurable against a specific defined test corpus, or is "100% of cases tested" undefined? [Measurability, Spec §SC-001] → **Fixed**: SC-001 now references "every codepoint in the U+00C0–U+00FF accented-Latin block per the data-model mapping table" instead of "é/ü/ñ/etc." — corpus is fully enumerable.
- [x] CHK023 — Is SC-002 ("T0/T1/T2 scores produced by same scoring method") testable from the JSON response alone, without inspecting internal state? [Measurability, Spec §SC-002] → **PASS**: SC-002 is verifiable via the `scoring_method` field in the public JSON response.
- [x] CHK024 — Does SC-004 (routing decisions match pre-change intent) reference a specific test set, or is "the calibration JD set" sufficiently defined in the spec? [Clarity, Spec §SC-004, Assumptions] → **Fixed**: SC-004 now references "the 12 JDs in `~/.local/share/go-apply/jd_cache/` used in the 2026-04-26 calibration run (see research.md Decision 6)." Decision 6 notes that the implementer MUST list filenames during T015 setup.

## Notes

- All 24 items resolved 2026-04-26 via council review.
- 19 items were already satisfied by the spec as written.
- 5 items required spec updates: CHK001 (FR-013), CHK017 (assumption expanded), CHK020 (FR-014), CHK022 (SC-001 corpus), CHK024 (SC-004 corpus reference).
