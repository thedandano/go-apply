# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Workflow
- **Parallel subagents**: Dispatch independent tasks as parallel subagents in worktree isolation. Use `sonnet` for hard/ambiguous tasks, `haiku` for straightforward/mechanical ones.
- **TDD/BDD**: Write failing tests before implementation. Use behavior-driven descriptions. Shift quality left.
- **Task completion**: Only mark a task `completed` when its PR is merged — not when the PR is created.
- **Commit discipline**: One commit per task, conventional commit format. Squash messy in-progress commits before opening a PR (`git rebase -i dev`).
- **Merge strategy**: feat branch → squash-merge into `dev` (PR for review). `dev` → rebase-merge into `main` (linear history, one commit per feature on main). Never use merge commits.
- **No hardcoded provider names**: Use generic names (`LLMClient`, `HTTPClient`). No provider names in code, comments, or tests.
- **No `interface{}` shortcuts**: Use precise types — concrete structs, typed constants, discriminated unions via type assertions.
- **Fix type errors properly**: Never suppress `go vet` or `staticcheck` warnings with blank identifiers or build tags unless there is no correct alternative.
- **Architecture invariant**: No service or repository package may import any presenter package. Dependency arrows only point inward through `internal/port/`.

## North Star
go-apply exists to help users land conversations with real humans — honestly. No fabricated experience, no invented skills. Every feature should make it easier for a user to present their genuine self more effectively, not to deceive.

## Project Overview
A Go CLI that scores resumes against job postings, tailors them via a two-tier cascade, generates cover letters, and operates in two modes: Headless (JSON for agents), MCP Server (Claude Code integration).

## Commands

```bash
# Build
make build              # outputs bin/go-apply
make install            # build + install to ~/.local/bin (override with INSTALL_DIR=...)

# Test
make test-unit          # go test -race ./internal/...
make test-integration   # go test -race -tags integration ./...
make test-e2e           # go test -race -tags e2e ./tests/e2e/...
make check              # vet + lint + security + test-unit (mirrors CI — run before pushing)

# Single test
go test -run TestFoo ./internal/service/pipeline/...

# Lint / format
make lint               # golangci-lint run ./...
make fmt                # goimports -w .
make vet                # go vet ./...
make security           # govulncheck + gosec

# Dev tools (run once after cloning)
make tools              # installs pinned golangci-lint, goimports, govulncheck, gosec
```

Tunable scoring constants (weights, thresholds, LLM params) live in `internal/config/defaults.json` — not in code.

## Architecture

### Layer diagram (dependency direction: inward only)

```
cmd/go-apply/main.go
  └── internal/cli/          ← Cobra commands (onboard, add_resume, get_config, serve, …)
  └── internal/mcpserver/    ← MCP stdio server; multi-turn session state machine
        └── internal/port/   ← interfaces (Scorer, Tailor, DocumentLoader, …)
              ├── internal/service/   ← implementations (scorer, tailor, pipeline, …)
              ├── internal/repository/fs/  ← filesystem adapters
              └── internal/presenter/mcp/  ← MCP envelope builder
```

`internal/port/` is the hexagonal boundary. Services and repositories depend only on `port` interfaces and `model` types. Presenters are outbound-only adapters; no inward package may import them.

### Operating mode

**MCP Server** (`go-apply serve`): `mcpserver/server.go` registers MCP tools. The MCP host (Claude) drives a multi-turn state machine over a disk-backed `SessionStore`. No `Orchestrator` is wired — Claude extracts keywords itself then calls `submit_keywords`. Sessions persist to `~/.local/share/go-apply/sessions/` and survive process restarts.

### MCP session state machine

```
load_jd → [stateLoaded]
  → submit_keywords → [stateScored]
      → submit_tailor_t1 → [stateT1Applied]
          → submit_tailor_t2 → [stateT2Applied]
      → preview_ats_extraction (any post-scored state)
      → finalize → [stateFinalized]
```

`requireOnboarded` middleware (`mcpserver/middleware.go`) gates workflow tools on a profile being present.

### Key packages

| Package | Role |
|---|---|
| `internal/port/` | All interfaces — the only cross-cutting dependency |
| `internal/model/` | Pure data types (no behaviour, no I/O) |
| `internal/service/pipeline/` | `AcquireJD`, `NewApplyPipeline` — shared pipeline primitives |
| `internal/service/tailor/` | Two-tier cascade: T1 (keyword injection) + T2 (LLM bullet rewrite) |
| `internal/service/scorer/` | Pure scoring; weights from `AppDefaults` |
| `internal/mcpserver/` | MCP server, session store, per-tool handlers |
| `internal/repository/fs/` | Filesystem adapters: resumes, JD cache, compiled profile, sessions |
| `internal/presenter/mcp/` | MCP response envelope builder |
| `internal/config/` | `Config` (YAML, XDG paths) + `AppDefaults` (defaults.json) |

## Development Guidelines
- Use proper error handling — wrap errors with `fmt.Errorf("context: %w", err)`.
- `golangci-lint` is configured in `.golangci.yml`; `gosec G304` is excluded globally; test files skip `gosec` and `errcheck`. `goimports` enforces the local prefix `github.com/thedandano/go-apply`.

## Testing
- Run all tests: `go test ./...`
- Run with race detector: `go test -race ./...`
- Run with coverage: `go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out`
- Build integration tests: `go test -tags integration ./...`
- Vet: `go vet ./...`

## Configuration
- Config file: `~/.config/go-apply/config.yaml`; XDG vars (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`) override defaults.
- API key: `GO_APPLY_API_KEY` env var (no config key — headless mode removed in 011).
- Data (resumes in `inputs/`, jd_cache/) stored in `~/.local/share/go-apply/`.
- **Migration**: `orchestrator.*` keys in `config.yaml` are now ignored; remove them. MCP mode has no LLM config — the host agent is the orchestrator.

<!-- SPECKIT START -->
## Active Feature Plan

**Feature**: 011-deprecate-headless-mcp-cleanup  
**Plan**: [specs/011-deprecate-headless-mcp-cleanup/plan.md](specs/011-deprecate-headless-mcp-cleanup/plan.md)  
**Spec**: [specs/011-deprecate-headless-mcp-cleanup/spec.md](specs/011-deprecate-headless-mcp-cleanup/spec.md)

See plan.md for task breakdown (parallel tracks A/B/C/D), agent assignments (sonnet/haiku), and verification steps.
<!-- SPECKIT END -->
