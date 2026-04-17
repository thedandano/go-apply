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

## Logging

Log files are written to `~/.local/state/go-apply/logs/go-apply-YYYY-MM-DD.log` — one file per calendar day (multiple invocations append); last 50 files retained.

```bash
# View recent logs
go-apply logs

# Watch live (tail -f equivalent)
go-apply logs --follow

# Show last 200 lines
go-apply logs --lines 200

# Tail the raw log file with grep
tail -f ~/.local/state/go-apply/logs/go-apply-$(date +%Y-%m-%d).log | grep ERROR
```

### Configuration

Log level and verbose mode are set in `~/.config/go-apply/config.yaml`:

```yaml
log_level: debug   # debug | info | warn | error (default: info)
verbose: true      # true = full request/response payloads in logs; false = truncated at 2 KB
```

Set them without editing the file directly:

```bash
go-apply config set log_level debug
go-apply config set verbose true
```

### MCP server debug logging

To enable debug logs when running as an MCP server, set `log_level` in the config file, or pass it via the `env` block in Claude Code's `settings.json` (config file is read on startup):

```json
{
  "mcpServers": {
    "go-apply": {
      "command": "go-apply",
      "args": ["serve"]
    }
  }
}
```

Run `go-apply config set log_level debug` once and every invocation — CLI and MCP — picks it up.

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
| `--text` | — | Raw JD text (mutually exclusive with `--url`) |
| `--headless` | `true` | JSON output mode (default; TUI coming in future) |
| `--channel` | `COLD` | Application channel: `COLD`, `REFERRAL`, `RECRUITER` |
| `--accomplishments` | — | Path to accomplishments doc for tier-2 bullet rewriting (optional) |

## MCP Server (Claude Code)

Add to Claude Code `settings.json`:
```json
{
  "mcpServers": {
    "go-apply": { "command": "go-apply", "args": ["serve"] }
  }
}
```

Available tools: `onboard_user`, `add_resume`, `get_config`, `update_config`, `load_jd`, `submit_keywords`, `submit_tailor_t1`, `submit_tailor_t2`, `finalize`

### MCP setup command

Instead of editing config files manually, use the setup command:

```bash
# Register with Claude Code
go-apply setup mcp --agent claude

# Register with OpenClaw
go-apply setup mcp --agent openclaw

# Register with Hermes
go-apply setup mcp --agent hermes

# Register with all known agents at once
go-apply setup mcp --agent all
```

To unregister:
```bash
go-apply setup mcp --agent claude --remove
go-apply setup mcp --agent all --remove
```

To overwrite an existing registration:
```bash
go-apply setup mcp --agent claude --override   # or --force (alias)
```

The command is idempotent — running it again without `--override` reports "already registered" and makes no changes. On a TTY you will be prompted to confirm the overwrite.

## CLI Reference

### Global (persistent) flags

These apply to every subcommand and must come before the subcommand name.

There are no persistent global flags. Log level and verbose mode are configured via `go-apply config set` (see [Logging](#logging)).

### `go-apply run`

Run the full apply pipeline against a job description.

| Flag | Default | Description |
|------|---------|-------------|
| `--url <url>` | — | URL of the job posting to fetch |
| `--text <jd>` | — | Raw job description text (mutually exclusive with `--url`) |
| `--headless` | `true` | JSON output mode (default; TUI coming in future) |
| `--channel <channel>` | `COLD` | Application channel: `COLD`, `REFERRAL`, `RECRUITER` |
| `--accomplishments <path>` | — | Path to accomplishments doc for tier-2 bullet rewriting (optional) |

### `go-apply serve`

Start the MCP stdio server for Claude Code integration. No flags.

### `go-apply onboard`

Store resumes, skills, and accomplishments in the profile database.

| Flag | Default | Description |
|------|---------|-------------|
| `--resume <path>` | — | Path to a resume file (repeatable; at least one required) |
| `--skills <path>` | — | Path to skills reference file (optional) |
| `--accomplishments <path>` | — | Path to accomplishments file (optional) |
| `--reset` | `false` | Delete profile database and `inputs/` directory |
| `--yes` | `false` | Skip confirmation prompt for `--reset` (required for non-interactive use) |

### `go-apply config`

Manage go-apply configuration. Subcommands:

| Subcommand | Usage | Description |
|------------|-------|-------------|
| `set` | `go-apply config set <key> <value>` | Set a config field by dot-notation key |
| `get` | `go-apply config get <key>` | Get a config field value by dot-notation key |
| `show` | `go-apply config show` | Show all config fields (API keys redacted) |

Config keys use dot notation (e.g. `llm.base_url`, `embedder.model`, `user_name`, `log_level`).

### `go-apply setup mcp`

Register or unregister go-apply as an MCP server in an AI agent's config.

| Flag | Default | Description |
|------|---------|-------------|
| `--agent <name>` | — | Agent to configure: `claude`, `openclaw`, `hermes`, `all` (required) |
| `--remove` | `false` | Unregister go-apply from the agent's config |
| `--override` | `false` | Overwrite an existing registration |
| `--force` | `false` | Alias for `--override` |

### `go-apply logs`

View recent go-apply log entries.

| Flag | Shorthand | Default | Description |
|------|-----------|---------|-------------|
| `--lines <n>` | `-n` | `100` | Number of recent lines to show |
| `--follow` | `-f` | `false` | Watch for new log lines (tail -f mode) |

### `go-apply update`

Update go-apply to the latest GitHub release. No flags.

> Note: cannot self-update a development build (`go install` builds).

### `go-apply version`

Print the go-apply version. No flags.

## Roadmap

| Feature | Status |
|---------|--------|
| Interactive TUI | Coming soon |
| Resume tailoring (keyword injection + bullet rewriting) | Coming soon |
| Multi-resume scoring with full breakdown | Available via `get_score` |
| MCP integration (Claude Code) | Available |
| Headless agent support (openclaw, hermes) | Available |
