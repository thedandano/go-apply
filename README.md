# go-apply

AI-powered job application CLI. Scores your resume against job postings, tailors it, and generates cover letters.

## Modes

| Mode | Command | Use case |
|------|---------|----------|
| Headless / Agent | `go-apply run --url <url>` | Scripts, openclaw, hermes |
| MCP Server | `go-apply serve` | Claude Code, openclaw, hermes |
| Interactive TUI | _(coming soon)_ | Human at terminal |

After installing, run `go-apply setup mcp --agent claude` to register with Claude Code.

## Installation

### Install script (recommended)

```bash
curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | bash
```

Install options (env vars):
```bash
# Install a specific version
curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | VERSION=0.1.0 bash

# Install to a custom directory (e.g., system-wide, requires sudo)
curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | INSTALL_DIR=/usr/local/bin sudo bash
```

To update, re-run the install command — it overwrites the existing binary.

### Uninstall

```bash
# Remove binary only (keeps config, data, and logs)
curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | bash -s -- --uninstall

# Remove everything (binary + config + data + logs)
curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | bash -s -- --uninstall --purge
```

### Homebrew (macOS/Linux)

```bash
brew install thedandano/tap/go-apply
```

### Go install

```bash
go install github.com/thedandano/go-apply/cmd/go-apply@latest
```

> Note: `go install` does not embed version info — `go-apply version` will show `dev`.

### From source

```bash
git clone https://github.com/thedandano/go-apply.git
cd go-apply
make build   # binary at bin/go-apply
```

## Configuration

Config file: `~/.config/go-apply/config.yaml`

```yaml
# CLI mode only — not needed for MCP (the host agent is the orchestrator)
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

> All tunable scoring constants (weights, thresholds, limits) live in `internal/config/defaults.json`.

## Logs

Each invocation writes a timestamped JSON log to `~/.local/state/go-apply/logs/`.
Format: `go-apply-2026-04-10T150405Z.log` — one file per run, last 50 retained.

```bash
# Watch latest run live
tail -f $(ls -t ~/.local/state/go-apply/logs/*.log | head -1) | jq .

# Filter errors only
cat ~/.local/state/go-apply/logs/go-apply-*.log | jq 'select(.level=="ERROR")'
```

## Logging

### Flags

| Flag | Shorthand | Description |
|------|-----------|-------------|
| `--log-level <level>` | | Set log level: `debug`, `info`, `warn`, `error` |
| `--debug` | `-v` | Enable debug logging (shorthand for `--log-level=debug`) |
| `--trace` | | Enable trace logging: debug level + full payload logging |

```bash
# Debug a single run
go-apply --debug run --url https://example.com/jobs/123

# Same via shorthand
go-apply -v run --url https://example.com/jobs/123

# Trace mode (debug + full request/response payloads)
go-apply --trace run --url https://example.com/jobs/123

# Set level explicitly
go-apply --log-level=warn run --url https://example.com/jobs/123
```

### Environment variables

| Variable | Description |
|----------|-------------|
| `GO_APPLY_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` |
| `GO_APPLY_LOG_VERBOSE` | Any non-empty value enables verbose/trace mode |

```bash
GO_APPLY_LOG_LEVEL=debug go-apply run --url https://example.com/jobs/123
GO_APPLY_LOG_VERBOSE=1 go-apply run --url https://example.com/jobs/123
```

### Precedence

Flag > environment variable > config file (`log_level`) > default (`INFO`)

### Log file location

`~/.local/state/go-apply/logs/go-apply-YYYY-MM-DD.log` — one file per day; last 50 retained.

### MCP server debug logging

To enable debug logs when running as an MCP server, add the `env` block to Claude Code's `settings.json`:

```json
{
  "mcpServers": {
    "go-apply": {
      "command": "go-apply",
      "args": ["serve"],
      "env": { "GO_APPLY_LOG_LEVEL": "debug" }
    }
  }
}
```

### Config file fallback

The `log_level` field in `~/.config/go-apply/config.yaml` sets a persistent default without needing a flag or env var each time. Flags and env vars always take precedence.

## Commands

### `go-apply run`

Run the full pipeline against a job description. Fetches (or accepts) the JD, scores all resumes in `~/.local/share/go-apply/inputs/`, augments resume text with profile context, and generates a cover letter.

```bash
# From a URL (fetches and caches the JD)
go-apply run --url https://example.com/jobs/123

# From raw text (useful in scripts or when the page is paywalled)
go-apply run --text "We are hiring a senior Go engineer..."

# Specify application channel
go-apply run --url <url> --channel REFERRAL   # COLD (default), REFERRAL, RECRUITER
```

**Output** (stdout, JSON):
```json
{
  "status": "success",
  "jd": { "title": "Senior Go Engineer", "company": "Acme", "required": ["go", "kubernetes"], ... },
  "scores": { "my-resume": { "breakdown": { "keyword_match": 40.5, ... }, ... } },
  "best_score": 82.3,
  "best_resume": "my-resume",
  "keywords": { "required": ["go", "kubernetes"], "preferred": ["docker"] },
  "cover_letter": { "text": "...", "channel": "COLD", "word_count": 180 },
  "start_time": "...", "end_time": "..."
}
```

Pipeline events (step-started/completed/failed) are written as JSON lines to stderr.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--url` | — | URL of the job posting |
| `--text` | — | Raw JD text (mutually exclusive with --url) |
| `--headless` | `true` | JSON output mode (default; TUI in future Epic 6) |
| `--channel` | `COLD` | Application channel: `COLD`, `REFERRAL`, `RECRUITER` |

## MCP Server (Claude Code)

Add to Claude Code `settings.json`:
```json
{
  "mcpServers": {
    "go-apply": { "command": "go-apply", "args": ["serve"] }
  }
}
```

Available tools: `get_score`, `onboard_user`, `add_resume`, `update_config`, `get_config`

### MCP setup command

Instead of editing config files manually, use the setup command:

```bash
# Register with Claude Code
go-apply setup mcp --agent claude

# Register with OpenClaw
go-apply setup mcp --agent openclaw

# Register with Hermes
go-apply setup mcp --agent hermes
```

To unregister:
```bash
go-apply setup mcp --agent claude --remove
```

The command is idempotent — running it again reports "already registered" and makes no changes.

## Roadmap

| Feature | Status |
|---------|--------|
| Interactive TUI | Coming soon |
| Resume tailoring (keyword injection + bullet rewriting) | Coming soon |
| Multi-resume scoring with full breakdown | Available via `get_score` |
| MCP integration (Claude Code) | Available |
| Headless agent support (openclaw, hermes) | Available |
