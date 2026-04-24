# go-apply — Job Application CLI

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

## Project Overview
A Go CLI that scores resumes against job postings, tailors them via a two-tier cascade, generates cover letters, and operates in three modes: TUI (interactive), Headless (JSON for agents), MCP Server (Claude Code integration).

## Development Guidelines
- Follow the port/adapter (hexagonal) architecture pattern
- Maintain existing code style and patterns
- Write unit tests for new functionality
- Keep dependencies updated via `go get`
- Use proper error handling — wrap errors with `fmt.Errorf("context: %w", err)`

## Testing
- Run all tests: `go test ./...`
- Run with race detector: `go test -race ./...`
- Run with coverage: `go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out`
- Build integration tests: `go test -tags integration ./...`
- Vet: `go vet ./...`

## Configuration
- Config loaded from `~/.config/go-apply/config.yaml`
- API key from `GO_APPLY_API_KEY` env var takes precedence over config file
- Data (resumes in `inputs/`, jd_cache/) stored in `~/.local/share/go-apply/`

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan at
`specs/002-preserve-finalize-logs/plan.md`
<!-- SPECKIT END -->
