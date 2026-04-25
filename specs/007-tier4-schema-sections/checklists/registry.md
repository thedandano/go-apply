# Section Registry Checklist: Tier 4 Schema Sections

**Purpose**: Author pre-commit self-review — validates requirements quality for US2 (section-registry refactor) with regression-risk focus on SC-006.
**Created**: 2026-04-25
**Feature**: [spec.md](../spec.md) · [contracts/section-registry.md](../contracts/section-registry.md)

---

## Requirement Completeness

- [ ] CHK001 — Are all 10 existing sections (Contact, Summary, Experience, Education, Skills, Projects, Certifications, Awards, Volunteer, Publications) listed in the registry contract in the correct order? [Completeness, contracts/section-registry.md]
- [ ] CHK002 — Are all 6 Tier 4 sections listed in the registry with their writer function signatures and exact heading strings (`LANGUAGES`, `SPEAKING ENGAGEMENTS`, `OPEN SOURCE`, `PATENTS`, `INTERESTS`, `REFERENCES`)? [Completeness, contracts/section-registry.md, Spec §FR-004]
- [ ] CHK003 — Is the `sectionWriter` struct definition (with `key string` and `write func(*strings.Builder, *model.SectionMap)`) specified in both the contract and the data-model? [Completeness, contracts/section-registry.md, data-model.md]
- [ ] CHK004 — Is the requirement that the dispatch loop contains NO hardcoded section-name strings explicitly stated as a code-review acceptance criterion (SC-003)? [Completeness, Spec §SC-003]
- [ ] CHK005 — Is the registry placement (package-level `var` or inline initialisation) specified, or is it deliberately left open to the implementer? [Completeness, contracts/section-registry.md]

## Requirement Clarity

- [ ] CHK006 — Is "byte-for-byte identical output" (SC-006) defined with enough precision to specify a concrete test strategy (golden file, string comparison, etc.)? Or is it left ambiguous how the comparison is performed? [Clarity, Spec §SC-006]
- [ ] CHK007 — Is the empty-section no-op invariant (Invariant 1 in the contract) stated in terms of what the writer MUST NOT emit (no empty heading line), not just what it does? [Clarity, contracts/section-registry.md §Invariants]
- [ ] CHK008 — Is the `key` field's purpose (informational in this spec; reserved for Spec C) clearly documented so an implementer doesn't add logic that branches on it? [Clarity, contracts/section-registry.md §Invariants]
- [ ] CHK009 — Is the Contact section special-cased clearly? (`writeContact` takes `*model.ContactInfo`, not a slice.) Is this distinction visible in the registry contract so it doesn't surprise the implementer? [Clarity, contracts/section-registry.md §Registry slice contract]

## Requirement Consistency

- [ ] CHK010 — Does the render order in the registry contract exactly match the order stated in `Spec §Assumptions` (Tier 4 appends after Publications: Languages → Speaking → Open Source → Patents → Interests → References)? [Consistency, Spec §Assumptions, contracts/section-registry.md]
- [ ] CHK011 — Is the empty-slice no-op rule consistent between the registry contract (Invariant 1), the spec Edge Cases, and the data-model validation rules? Are there any contradictions? [Consistency, Spec §Edge Cases, data-model.md, contracts/section-registry.md]
- [ ] CHK012 — Are the heading strings in `contracts/section-registry.md §Tier 4 heading strings` consistent with those stated in `Spec §FR-004`? [Consistency, Spec §FR-004, contracts/section-registry.md]

## Acceptance Criteria Quality

- [ ] CHK013 — Is SC-006 ("byte-for-byte identical output") achievable as stated? Is there a known test fixture for pre-refactor output, or does the spec need to define how the baseline is established? [Measurability, Spec §SC-006]
- [ ] CHK014 — Is SC-003 ("no hardcoded section-name strings in dispatch loop") objectively verifiable by code review alone, or should a static analysis check be specified (e.g., grep for known section header strings)? [Measurability, Spec §SC-003]
- [ ] CHK015 — Does US2's Independent Test ("code review confirms no hardcoded section-name strings") constitute a sufficient acceptance signal on its own, or does it need a companion runtime test? [Measurability, Spec §User Story 2 — Independent Test]

## Scenario Coverage

- [ ] CHK016 — Are requirements defined for a resume where the `Order []string` field in `SectionMap` lists a Tier 4 key? Is the interaction between `Order` and the registry render sequence specified? [Coverage, Gap]
- [ ] CHK017 — Are requirements defined for a resume with all 16 sections populated simultaneously? Is there a requirement that section order is stable across repeated renders of the same input? [Coverage, Spec §SC-001]
- [ ] CHK018 — Are requirements defined for the Summary section, which is a `string` not a slice — does its empty-check rule (`sections.Summary != ""`) appear in the contract alongside the slice-based no-op rule? [Coverage, contracts/section-registry.md]

## Edge Case Coverage

- [ ] CHK019 — Is the behaviour defined when the registry slice itself is empty (zero entries)? Should `Render` return empty string or an error? [Edge Case, Gap]
- [ ] CHK020 — Is there a requirement that the registry slice is immutable after initialisation — i.e., no runtime modification? If not, is concurrent access a concern that should be stated? [Edge Case, Spec §Assumptions]
- [ ] CHK021 — Is the Contact section's non-slice treatment (always emitted, no empty guard) consistent with the general "empty = no-op" rule? Is this exception documented? [Edge Case, contracts/section-registry.md]

## Non-Functional Requirements

- [ ] CHK022 — Is there a performance requirement for `Render` after the registry refactor? Or is it explicitly N/A (as stated in plan.md Technical Context — no hot paths affected)? [NFR, plan.md §Technical Context]
- [ ] CHK023 — Is the registry initialisation strategy (package-level vs. per-call) specified as having no allocation-per-render overhead requirement? [NFR, Gap]

## Dependencies & Assumptions

- [ ] CHK024 — Is the assumption that `preview_ats_extraction` requires no change (section-agnostic handler) validated by referencing the handler's actual implementation path, not just asserted? [Assumption, Spec §FR-008]
- [ ] CHK025 — Is the out-of-scope boundary for the FR-011 order discrepancy (`Experience → Education → Skills`) explicitly documented so that a reviewer does not flag it as a bug introduced by the registry refactor? [Assumption, Spec §Assumptions]
- [ ] CHK026 — Is the assumption that the existing `writeX` private functions are preserved as-is (not merged into closures) stated, so the refactor's blast radius is clear? [Dependency, contracts/section-registry.md]

## Notes

- Check items off as completed: `[x]`
- CHK013 and CHK016 are flagged `[Gap]` — consider adding explicit baseline test strategy and `Order` field behaviour to the spec before writing tests.
- Cross-reference: data-model + extractor checklist is at `checklists/data-model.md`.
