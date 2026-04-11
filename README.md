# go-apply

AI-powered job application CLI. Scores your resume against job postings, tailors it, and generates cover letters.

## Modes

| Mode | Command | Use case |
|------|---------|----------|
| Interactive TUI | `go-apply apply --url <url>` | Human at terminal |
| Headless / Agent | `go-apply apply --headless --url <url>` | Scripts, openclaw, hermes |
| MCP Server | `go-apply serve` | Claude Code integration |

## Installation

### Homebrew (macOS/Linux)
```bash
brew install thedandano/tap/go-apply
```

### Download binary
See [Releases](https://github.com/thedandano/go-apply/releases) for pre-built binaries.

### From source
```bash
go install github.com/thedandano/go-apply/cmd/go-apply@latest
```

## Configuration

Config file: `~/.config/go-apply/config.yaml`

```yaml
orchestrator:
  base_url: https://api.anthropic.com/v1
  model: claude-sonnet-4-6
  api_key: sk-ant-...        # or set GO_APPLY_API_KEY env var

embedder:
  base_url: http://localhost:11434/v1
  model: nomic-embed-text
  api_key: ""
embedding_dim: 768

years_of_experience: 7
default_seniority: senior
user_name: "Your Name"
occupation: "Software Engineer"
location: "San Francisco, CA"
```

> All tunable scoring constants (weights, thresholds, limits) live in `config/defaults.json`.

## Commands

*(expanded as features are delivered)*

## MCP Server (Claude Code)

Add to Claude Code `settings.json`:
```json
{
  "mcpServers": {
    "go-apply": { "command": "go-apply", "args": ["serve"] }
  }
}
```

Available tools: `apply_to_job`, `tailor_resume`, `get_score`
