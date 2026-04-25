# PR Review Gate Checklist: Pretty Log Output

**Purpose**: Validate that requirements in spec.md and plan.md are complete, clear, consistent, and specific enough to verify the implementation at PR review time. These are unit tests for the requirements — not implementation tests.
**Created**: 2026-04-25
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md)
**Audience**: Peer reviewer during PR review

---

## Requirement Completeness

- [ ] CHK001 — Is the exact visual output format (indentation characters, label format) fully specified in requirements, or only implied by the code-block example in User Story 1? [Completeness, Spec §US-1]
- [ ] CHK002 — Are requirements defined for the case where a log line contains ONLY JSON-valued fields and no non-JSON key=value pairs remain on the header line? [Gap, Completeness]
- [ ] CHK003 — Are requirements specified for the behavior when the output writer returns an error during rendering (e.g., broken pipe)? [Gap, Exception Flow]
- [ ] CHK004 — Is the scope of "all log levels" in FR-006 consistent with charmbracelet/log's actual level labels (DEBU, INFO, WARN, ERRO)? [Completeness, Spec §FR-006]

## Requirement Clarity

- [ ] CHK005 — Is the indentation hierarchy (header line → `  key:` label → `    JSON content`) specified with exact character counts, or only illustrated by example? [Clarity, Spec §FR-001, §FR-004]
- [ ] CHK006 — Is "quoted string field value" in FR-001 defined precisely enough to distinguish Go-quoted strings (produced by charmbracelet/log) from other quoting conventions? [Clarity, Spec §FR-001]
- [ ] CHK007 — Does FR-002 ("preserve all non-JSON fields exactly") define the expected whitespace behavior when a JSON field is removed from the middle of a line? [Clarity, Spec §FR-002]
- [ ] CHK008 — Is it explicit in FR-001 or the assumptions that JSON primitives (strings, numbers, booleans, `null`) do NOT trigger pretty-printing — only objects and arrays do? [Clarity, Gap]

## Requirement Consistency

- [ ] CHK009 — Does the display format in User Story 1 Acceptance Scenario 1 (the code-block example) align precisely with FR-001 and FR-004's indentation specification? [Consistency, Spec §US-1, §FR-001, §FR-004]
- [ ] CHK010 — Is FR-007 ("underlying log storage MUST NOT be modified") consistent with the stated assumption that `go-apply logs` is a read-only operation on log files? [Consistency, Spec §FR-007, §Assumptions]
- [ ] CHK011 — Does FR-008 (`--follow` real-time rendering) align with User Story 1 Acceptance Scenario 5 and the `go-apply logs --follow` feature behavior described in the spec? [Consistency, Spec §FR-008, §US-1]
- [ ] CHK012 — Are the three clarification answers recorded in the Clarifications section accurately reflected in the corresponding FR and SC items throughout the spec? [Consistency, Spec §Clarifications]

## Acceptance Criteria Quality

- [ ] CHK013 — Is SC-002 ("100% of entries containing valid JSON displayed with consistent indentation") verifiable during a PR review without running the full log corpus? [Measurability, Spec §SC-002]
- [ ] CHK014 — Is SC-005 ("no observable delay") measurable with a specific threshold or benchmark, or is it too vague to serve as a pass/fail PR gate criterion? [Clarity, Spec §SC-005]
- [ ] CHK015 — Are SC-001 through SC-005 each traceable to a specific FR, so a reviewer can confirm each criterion validates exactly one requirement? [Traceability]
- [ ] CHK016 — Is SC-004 ("raw log source byte-for-byte identical") achievable and verifiable given the renderer only operates at display time and never touches the log file? [Consistency, Spec §SC-004]

## Scenario Coverage

- [ ] CHK017 — Are requirements defined for a JSON-valued field that appears in the middle of a line (between other key=value pairs), ensuring the header reconstruction is unambiguous? [Coverage, Spec §FR-002]
- [ ] CHK018 — Are requirements specified for the `--follow` scenario where a new log entry arrives while a previous multi-line JSON block is still being written? [Coverage, Gap]
- [ ] CHK019 — Is the behavior of pretty-printing specified to be consistent regardless of which log file `go-apply logs` selects (oldest, newest, single file)? [Coverage, Spec §Assumptions]

## Edge Case Coverage

- [ ] CHK020 — Is the behavior explicitly specified for a quoted field value that is a valid JSON primitive (`result="42"`, `result="true"`) — does it stay on the header line or trigger pretty-printing? [Clarity, Edge Case, Gap]
- [ ] CHK021 — Is the behavior defined for a double-encoded JSON string (a quoted value whose content is itself a JSON-encoded string rather than an object or array)? [Edge Case, Spec §Edge Cases]
- [ ] CHK022 — Are requirements defined for empty JSON containers (`result="{}"`, `result="[]"`) — do they trigger the split layout or stay on the header line? [Edge Case, Gap]
- [ ] CHK023 — Is the behavior specified for a log line that is entirely empty or whitespace-only (e.g., blank separator lines in the log file)? [Edge Case, Spec §Edge Cases]

## Non-Functional Requirements

- [ ] CHK024 — Are memory usage requirements defined for rendering very large JSON payloads (thousands of fields), or is this explicitly deferred with a rationale? [Completeness, Spec §Edge Cases]
- [ ] CHK025 — Is there a requirement specifying whether output must be buffered per-entry to prevent partial line interleaving in `--follow` mode under concurrent writes? [Gap, Reliability]

## Dependencies & Assumptions

- [ ] CHK026 — Is the assumption that charmbracelet/log always produces Go `strconv.Quote`-formatted string values documented and validated against the actual library source or tests? [Assumption, Spec §Assumptions]
- [ ] CHK027 — Are requirements scoped explicitly to the current logfmt format, with a note on what happens if the log format changes (e.g., future switch to structured JSON logging)? [Assumption, Gap]

## Notes

- Check items off as completed: `[x]`
- Add findings inline: `- [x] CHK005 — Confirmed: plan.md D-004 specifies exact indentation`
- Items marked `[Gap]` indicate potentially missing requirements — resolve in spec before merge
- Items marked `[Assumption]` require validation against library source or existing tests
