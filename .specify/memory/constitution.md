<!--
Sync Impact Report
Version change: 1.0.0 → 1.1.0
Modified principles: V. Observability — marked NON-NEGOTIABLE, scope clarified (logging + metrics +
  traceability, not logging alone)
Added sections: none
Removed sections: none
Templates reviewed: no updates required
Deferred TODOs: none
-->

# go-apply Constitution

## Core Principles

### I. Vertical Slicing (NON-NEGOTIABLE)

Every feature MUST be delivered as a complete, independently deployable vertical slice.
Each slice delivers observable user value end-to-end — no horizontal layers shipped in isolation.
User stories are the unit of delivery; each story MUST be independently testable and demonstrable
without requiring other stories to be complete. A slice is done when its tests pass, CI is green,
and it is merged to `dev`.

**Rationale**: Horizontal layers (e.g., "data model only") accumulate integration risk and block
feedback. Vertical slices let each increment be shipped, tested, and validated against real
behavior at every stage of development.

### II. Test-First Development (NON-NEGOTIABLE)

Tests MUST be written before implementation (Red → Green → Refactor).
Behavior MUST be described in Given/When/Then scenarios before any code is written.
Minimum 80% code coverage is enforced at pre-commit.

Test layers:
- **Unit**: fast, deterministic, component-level — no I/O
- **Integration**: validate end-to-end flows across components
- **E2E**: run before deployment; validate realistic system behavior

No PR may be merged with failing tests. No skipping tests under any circumstance.

**Rationale**: Tests are the primary design tool. Writing them first forces clear interface
contracts, exposes hidden complexity early, and ensures regressions are caught before they
accumulate.

### III. Hexagonal Architecture (NON-NEGOTIABLE)

The port/adapter (hexagonal) pattern governs all structural decisions.
Dependency arrows MUST point inward through `internal/port/`.
No service or repository package may import any presenter package.
Components MUST be replaceable without cascading changes across the codebase.

Additional invariants:
- No hardcoded provider names in code, comments, or tests — use generic names (`LLMClient`,
  `HTTPClient`)
- No `interface{}` shortcuts — use precise types, typed constants, discriminated unions via
  type assertions
- `go vet` and `staticcheck` warnings MUST NOT be suppressed with blank identifiers or build tags
  unless no correct alternative exists

**Rationale**: Hexagonal architecture enforces separation of concerns, makes components
independently testable, and prevents the accidental coupling that makes systems brittle.

### IV. No Silent Failures (NON-NEGOTIABLE)

All errors MUST be explicit. No swallowing errors. No bare `_` on error returns in production code.
Fail fast: stop execution on invalid conditions rather than degrading silently.
All fallbacks MUST be explicitly defined, approved, and observable in logs.
No implicit fallbacks, no simulating missing dependencies.

Error handling standard: `fmt.Errorf("context: %w", err)` — always wrap with context.

**Rationale**: Silent failures create invisible production incidents. Fail-fast surfaces problems
at the earliest possible moment, when they are cheapest to fix and easiest to diagnose.

### V. Observability (NON-NEGOTIABLE)

Observability is more than logging. It is the property that makes a system's behavior
understandable from its outputs alone — without attaching a debugger or reading source code.

It covers three mandatory layers:
- **Structured logging**: every critical operation MUST emit a structured log entry (using `slog`)
  containing: operation name, inputs (when safe), outputs, errors, and context identifiers
- **Traceability**: operations that span multiple components MUST carry a correlation identifier
  (e.g., session ID) so a complete request can be reconstructed from logs alone
- **Debug visibility**: verbose mode MUST surface intermediate state (diffs, payloads, scoring
  breakdowns) via `logger.Verbose()` gates — never by default, always on demand

Behavior MUST be fully debuggable from log output alone. Hidden state is a defect.

CI pipelines MUST enforce:
- Linting and formatting
- Unit tests and integration tests
- Security scanning and static analysis
- CI MUST fail on any issue — no merging with failing checks

Pre-commit hooks MUST run: linting, formatting, static checks, 80% coverage gate.
Pre-push hooks MUST run: unit tests.

**Rationale**: A system that fails silently, or that requires source-level inspection to debug,
cannot be operated reliably in production. Observability is a correctness property, not an
add-on.

## Development Workflow

**Branching and merging**:
- Feature branch → squash-merge into `dev` (PR required for review)
- `dev` → rebase-merge into `main` (linear history, one commit per feature on main)
- Never use merge commits

**Commit discipline**:
- One commit per task, conventional commit format (`feat:`, `fix:`, `chore:`, etc.)
- Squash messy in-progress commits before opening a PR (`git rebase -i dev`)
- `feat:` = minor version bump; `fix:` = patch bump — do not mix `fix:` commits into a `feat:` PR

**Agent workflow**:
- Dispatch independent tasks as parallel subagents in worktree isolation
- Use `sonnet` for hard/ambiguous tasks, `haiku` for mechanical/straightforward ones
- Mark tasks `completed` only when the PR is merged — not when the PR is created

**PR blocking rule**: Wait for a PR to merge before opening the next one if they touch the same
files. Concurrent PRs on shared files cause avoidable conflicts.

## Governance

This constitution supersedes all other practices. In the event of a conflict between this
document and any other guidance (README, CLAUDE.md, agent instructions), this constitution
takes precedence.

**Amendment procedure**:
1. Document the proposed change and its rationale
2. Identify which principles are affected and any migration plan required
3. Update this file and bump the version per the versioning policy below
4. Propagate changes to any dependent templates or guidance files

**Versioning policy**:
- MAJOR: principle removal, redefinition, or backward-incompatible governance change
- MINOR: new principle or section added, or materially expanded guidance
- PATCH: clarifications, wording, typo fixes, non-semantic refinements

**Compliance**: All PRs MUST verify compliance with Principles I–V before merge.
The Constitution Check section in `plan-template.md` gates each feature plan on this document.

**Version**: 1.1.0 | **Ratified**: 2026-04-23 | **Last Amended**: 2026-04-23
