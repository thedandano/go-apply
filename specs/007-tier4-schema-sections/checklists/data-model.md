# Data Model & Extractor Interface Checklist: Tier 4 Schema Sections

**Purpose**: Author pre-commit self-review — validates requirements quality for US1 (Tier 4 model additions) and US3 (port.Extractor signature fix) before implementation begins.
**Created**: 2026-04-25
**Feature**: [spec.md](../spec.md) · [data-model.md](../data-model.md) · [contracts/extractor-interface.md](../contracts/extractor-interface.md)

---

## Requirement Completeness

- [ ] CHK001 — Are all six Tier 4 section types represented in `SectionMap` with typed slice fields (not generic `[]any`)? [Completeness, Spec §FR-001]
- [ ] CHK002 — Does each new entry struct have the domain-appropriate fields defined in `data-model.md` (`LanguageEntry.Proficiency`, `SpeakingEntry.Event`, `OpenSourceEntry.Role`, `PatentEntry.Number`, `ReferenceEntry.Contact`)? [Completeness, Spec §Key Entities]
- [ ] CHK003 — Are all six new section keys (`languages`, `speaking`, `open_source`, `patents`, `interests`, `references`) listed in the `knownSections` allowlist requirement? [Completeness, Spec §FR-002]
- [ ] CHK004 — Is the requirement that `parseSectionsArg` accepts the six new keys explicitly stated and traceable to a testable assertion? [Completeness, Spec §FR-005]
- [ ] CHK005 — Are all four `port.Extractor` call-site files identified and documented as requiring update? (`port/extract.go`, `service/extract/extract.go`, `service/extract/extract_test.go`, `mcpserver/session_tools.go`) [Completeness, Spec §FR-006, contracts/extractor-interface.md]
- [ ] CHK006 — Is the stub `Extract([]byte)` return behaviour (identity: `string(data)`) specified in the contracts? [Completeness, contracts/extractor-interface.md]

## Requirement Clarity

- [ ] CHK007 — Are JSON and YAML tag conventions for new fields specified (`json:"open_source,omitempty" yaml:"open_source,omitempty"`)? Is the underscore key for `open_source` explicit to avoid ambiguity with `OpenSource` Go field name? [Clarity, data-model.md]
- [ ] CHK008 — Is `InterestEntry` with a single `Name string` field clearly justified over `[]string`? Is the reasoning documented so a reviewer doesn't flag it as an oversight? [Clarity, Spec §Key Entities]
- [ ] CHK009 — Is the `Extract(data []byte)` signature change unambiguous — specifically, are callers documented to use explicit `[]byte(s)` casts rather than a helper wrapper? [Clarity, contracts/extractor-interface.md]
- [ ] CHK010 — Is the `open_source` JSON key name documented as intentional (underscore, not hyphen or camelCase), and is it consistent with the `knownSections` key string `"open_source"`? [Clarity, data-model.md, Spec §FR-002]

## Requirement Consistency

- [ ] CHK011 — Are `SectionMap` JSON field names (`"open_source"`, `"speaking"`, etc.) consistent with the `knownSections` allowlist key strings and the `parseSectionsArg` accepted keys? [Consistency, Spec §FR-001, §FR-002, §FR-005]
- [ ] CHK012 — Do new entry struct field names follow the same convention as existing entries (`PublicationEntry`: `Title`, `Venue`, `Date`, `URL`)? Are there any deviations (e.g., `SpeakingEntry.URL` vs `SpeakingEntry.Url`)? [Consistency, data-model.md]
- [ ] CHK013 — Is the `omitempty` requirement applied uniformly to all new fields on both JSON and YAML tags? [Consistency, data-model.md]
- [ ] CHK014 — Does the "no per-field required validation" rule apply equally to all six Tier 4 entry structs, with no exceptions documented? [Consistency, Spec §Clarifications, data-model.md §Validation rules]

## Acceptance Criteria Quality

- [ ] CHK015 — Is SC-001 ("zero sections silently dropped") measurable — i.e., does it specify a test fixture that includes all six Tier 4 sections and asserts each heading appears in output? [Measurability, Spec §SC-001]
- [ ] CHK016 — Is SC-004 (`go test -race` green) a sufficient acceptance gate for the JSON round-trip requirement, or does the spec need an explicit round-trip test requirement? [Measurability, Spec §SC-004]
- [ ] CHK017 — Is SC-005 (`go vet && go build` clean) sufficient to validate the `Extract([]byte)` signature change, or should the spec explicitly require a compile-time type-check assertion? [Measurability, Spec §SC-005]

## Scenario Coverage

- [ ] CHK018 — Are requirements defined for a resume that has SOME but not ALL Tier 4 sections (e.g., only `Patents` and `Languages`)? Is the rendering of absent sections (no-op) explicitly stated? [Coverage, Spec §Edge Cases]
- [ ] CHK019 — Are requirements defined for the case where a Tier 4 key appears in a `SectionMap` that is loaded from existing YAML that predates this spec (i.e., YAML without Tier 4 fields)? Is backward-compatibility on load specified? [Coverage, Spec §Assumptions]
- [ ] CHK020 — Is the behavior of `parseSectionsArg` when receiving a Tier 4 key alongside currently-unknown keys specified (does it reject the entire input or just the unknown key)? [Coverage, Spec §Edge Cases]

## Edge Case Coverage

- [ ] CHK021 — Is the empty-slice-is-valid rule (omitempty, no-op writer) explicitly traceable from `SectionMap` definition → validation rules → writer contract? [Edge Case, Spec §Edge Cases, data-model.md §Validation rules]
- [ ] CHK022 — Is the behavior for a Tier 4 entry with all empty string fields defined? (Per clarification: valid, no per-field validation.) Is this documented in both spec and data-model? [Edge Case, Spec §Clarifications]
- [ ] CHK023 — Are requirements defined for a `ReferenceEntry.Contact` field that contains PII (phone/email)? Is it noted that `redact.RedactAny` handles it automatically via reflection? [Edge Case, Gap]

## Dependencies & Assumptions

- [ ] CHK024 — Is the assumption that `redact.RedactAny` handles new structs automatically (via reflection over exported string fields) explicitly stated, or does it rely on implicit knowledge of the redactor implementation? [Assumption, Spec §Assumptions]
- [ ] CHK025 — Is the assumption that `preview_ats_extraction` requires no handler change documented with a clear rationale (section-agnostic handler + FR-008)? Could a reviewer misread this as a gap? [Assumption, Spec §FR-008]
- [ ] CHK026 — Are the four files in the `Extract([]byte)` blast radius documented such that a reviewer can verify all callers were updated without reading the code? [Dependency, contracts/extractor-interface.md]

## Notes

- Check items off as completed: `[x]`
- Items marked `[Gap]` indicate requirements that may need to be added to the spec before implementation.
- Cross-reference: registry checklist is at `checklists/registry.md`.
