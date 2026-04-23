# go-apply — Developer Guide

## High-level architecture

go-apply is a substrate, not an AI model. It provides the load, score, persist, and
prompt layers of a job application workflow. An MCP agent (Claude Code, Hermes,
Openclaw) or a human-authored script provides the tailored resume text; go-apply
stores it, rescores it, and persists the record.

Core packages:

```
cmd/go-apply/main.go          CLI entry point
internal/cli/                 Cobra commands — wire dependencies and call services
internal/port/                Interface definitions (the contracts)
internal/service/pipeline/    Orchestration — calls services, emits Presenter events
internal/service/scorer/      Deterministic scoring algorithm (pure Go, no I/O)
internal/service/fetcher/     JD web fetching (chromedp primary, goquery fallback)
internal/service/coverletter/ Cover letter generation
internal/repository/fs/       File system repos (resume files, JD cache, sessions)
internal/presenter/headless/  JSON output (scripted/headless mode)
internal/presenter/mcp/       MCP tool result accumulator
internal/mcpserver/           MCP stdio server (tools + tailor_resume prompt)
internal/mcpserver/skills/    Vendored skill artifacts (refreshed via `make sync-skill`)
internal/config/defaults.json All tunable constants — edit here, not in source
```

The tailoring contract lives in `internal/mcpserver/skills/resume-tailor.md` —
a vendored copy of the grimoire's `resume-tailor` skill embedded at build time
via `//go:embed`. `make sync-skill` atomically replaces it from the source tree
and regenerates the `.sha256` integrity sentinel. `TestTailorSkillBodyIntegrityHash`
catches embedded/sentinel mismatches in CI.

## Adding a New Adapter

To swap any external dependency (web fetcher, file store, presenter):

1. Implement the relevant `port.*` interface in a new package under `internal/service/` or `internal/repository/`
2. Replace the constructor call in `internal/cli/*.go` — one line change
3. Add unit tests using the interface, not the concrete type

## Running Locally

### MCP mode (primary)

Register the server and let Claude Code drive the workflow:

```bash
go-apply setup mcp --agent claude
# Then ask Claude Code: "Score my resume against this job and tailor it"
```

### Headless CLI (scripted)

Chain the four subcommands directly:

```bash
# 1. Load the job description
SESSION=$(go-apply load-jd --url https://example.com/jobs/123 | jq -r .session_id)

# 2. Score resumes
go-apply score --session "$SESSION"

# 3. Tailor the resume (you author or generate the text externally)
go-apply submit-tailored-resume --session "$SESSION" --file tailored.md

# 4. Persist the record
go-apply finalize --session "$SESSION"
```

## Running Tests

```bash
make test-unit        # fast, no I/O — run before every commit
make test-integration # FS integration tests — run before every PR
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

Available tools: `onboard_user`, `add_resume`, `get_config`, `update_config`,
`load_jd`, `submit_keywords`, `submit_tailored_resume`, `finalize`

Available prompts: `tailor_resume` — fetches the embedded `resume-tailor` skill
plus a go-apply-specific prelude; the agent follows it and calls `submit_tailored_resume`
when done.
