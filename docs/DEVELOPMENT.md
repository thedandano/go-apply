# go-apply — Developer Guide

## Architecture

go-apply uses a port/adapter (hexagonal) architecture. Business logic is isolated in
`internal/service/` and `internal/repository/`. All I/O (output rendering, LLM calls,
file access, web fetching) is behind interfaces defined in `internal/port/`.

```
cmd/go-apply/main.go          CLI entry point
internal/cli/                 Cobra commands — wire dependencies and call pipelines
internal/port/                Interface definitions (the contracts)
internal/service/pipeline/    Orchestration — calls services, emits Presenter events
internal/service/scorer/      Deterministic scoring algorithm (pure Go, no I/O)
internal/service/llm/         LLM + embedding HTTP client (OpenAI-compatible)
internal/service/fetcher/     JD web fetching (chromedp primary, goquery fallback)
internal/service/tailor/      Resume tailoring (tier1 keywords, tier2 bullet rewrites)
internal/service/coverletter/ Cover letter generation
internal/repository/fs/       File system repos (resume files, JD cache)
internal/repository/sqlite/   SQLite + sqlite-vec (profile embeddings)
internal/presenter/headless/  JSON output (agent/headless mode)
internal/presenter/mcp/       MCP tool result accumulator
tui/                          bubbletea TUI (Epic 6)
internal/config/defaults.json All tunable constants — edit here, not in source
```

## Adding a New Adapter

To swap any external dependency (LLM provider, vector store, TUI framework):

1. Implement the relevant `port.*` interface in a new package under `internal/service/` or `internal/repository/`
2. Replace the constructor call in `internal/cli/*.go` — one line change
3. Add unit tests using the interface, not the concrete type

## Running Tests

```bash
make test-unit        # fast, no I/O — run before every commit
make test-integration # real SQLite + FS — run before every PR
make test-e2e         # builds binary, runs full CLI — run before every PR
```

## No Magic Numbers

All tunable constants live in `internal/config/defaults.json`. The `AppDefaults` struct
(`internal/config/defaults.go`) mirrors that JSON. If you change a value in one,
change it in both — `TestDefaultsMatchJSON` will catch mismatches in CI.

## PR Workflow

1. Branch: `feat/task-N-description`
2. Implement with TDD (red → green → refactor)
3. Run `make test-unit test-integration lint`
4. Open PR — one task per PR, small and focused
5. Code reviewer leaves inline comments
6. Author resolves comments
7. User reviews and approves
8. Merge after both reviews pass

## Release

```bash
git tag v0.X.0
git push origin v0.X.0
# GitHub Actions release.yml runs goreleaser
```

## MCP Integration (Claude Code)

Add to Claude Code `settings.json`:
```json
{
  "mcpServers": {
    "go-apply": {
      "command": "/usr/local/bin/go-apply",
      "args": ["serve"]
    }
  }
}
```

Available tools: `apply_to_job`, `tailor_resume`, `get_score`
