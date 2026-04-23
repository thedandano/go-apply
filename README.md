# go-apply

go-apply is a substrate — loader, scorer, prompt, and persister — that an MCP agent drives. It fetches job descriptions, scores resumes deterministically, embeds the resume-tailor skill prompt for the agent, accepts the agent's rewritten resume via `submit_tailored_resume`, and persists the application record. go-apply does not make LLM calls or produce tailored text on its own.

[![CI](https://github.com/thedandano/go-apply/actions/workflows/ci.yml/badge.svg)](https://github.com/thedandano/go-apply/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/thedandano/go-apply)](https://github.com/thedandano/go-apply/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/thedandano/go-apply)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/thedandano/go-apply)](https://goreportcard.com/report/github.com/thedandano/go-apply)
[![Works with Claude Code](https://img.shields.io/badge/Works%20with-Claude%20Code-blueviolet?logo=anthropic)](https://claude.ai/code)
[![Works with OpenClaw](https://img.shields.io/badge/Works%20with-OpenClaw-orange)](https://github.com/openclaw)
[![Works with Hermes](https://img.shields.io/badge/Works%20with-Hermes-teal)](https://github.com/hermes-agent)

## Modes

| Mode | Command | Use case |
|------|---------|----------|
| **MCP** (primary) | `go-apply serve` | Claude Code, Hermes, Openclaw — the agent drives the full tool flow |
| **Headless CLI** | `go-apply load-jd / score / submit-tailored-resume / finalize` | Scripted pipelines without MCP |

## Quick Start

1. **Install**
   ```bash
   curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | bash
   ```

2. **Register with your MCP agent**
   ```bash
   go-apply setup mcp --agent claude    # Claude Code
   go-apply setup mcp --agent hermes    # Hermes
   go-apply setup mcp --agent openclaw  # Openclaw
   ```

3. **Ask your agent to onboard and apply** — the agent drives the full flow via MCP tools:
   - *"Onboard my resume at ~/docs/resume.md"*
   - *"Score my resume against this job posting and tailor it"*

   The agent reads the `tailor_resume` prompt (which embeds the `resume-tailor` skill verbatim), writes the full tailored resume, and submits it via `submit_tailored_resume`. go-apply rescores and persists the result.

   The happy path is: `load_jd → submit_keywords → submit_tailored_resume → finalize`.

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

## Configuration

Config file: `~/.config/go-apply/config.yaml`

```yaml
user_name: "Your Name"
occupation: "Software Engineer"
location: "San Francisco, CA"
linkedin_url: "https://linkedin.com/in/yourname"
years_of_experience: 7
log_level: info    # debug | info | warn | error
verbose: false     # true = full request/response payloads in logs
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

## MCP Server (Claude Code, Hermes, Openclaw)

Add to Claude Code `settings.json`:
```json
{
  "mcpServers": {
    "go-apply": { "command": "go-apply", "args": ["serve"] }
  }
}
```

### Available tools

| Tool | Description |
|------|-------------|
| `onboard_user` | Store a resume, skills, and accomplishments into the profile database |
| `add_resume` | Add or replace a single resume in the profile database |
| `get_config` | Return all go-apply config fields (API keys redacted) |
| `update_config` | Set a config field by dot-notation key |
| `load_jd` | Fetch the job description by URL or accept raw text; returns `session_id` |
| `submit_keywords` | Submit extracted keywords to score resumes; returns scores and `next_action` |
| `submit_tailored_resume` | Accept the agent's full rewritten resume, rescore, and advance the session |
| `finalize` | Persist the application record and close the session |

### Available prompts

| Prompt | Description |
|--------|-------------|
| `tailor_resume` | Embeds the `resume-tailor` skill verbatim plus a go-apply-specific prelude. The agent reads this prompt, writes the full tailored resume, and submits it via `submit_tailored_resume`. |

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

## Scripted usage (headless)

The CLI does not produce tailored resume text. It accepts tailored text as input. You still need Claude Code, another agent, or a manual rewrite to author the file passed to `submit-tailored-resume`. The CLI is useful for scripting the load/score/persist sides of the workflow around a tailoring step you produce elsewhere.

```bash
# 1. Load the job description and capture the session ID
SESSION=$(go-apply load-jd --url https://example.com/jobs/123 | jq -r .session_id)

# 2. Score resumes against the extracted keywords
go-apply score --session "$SESSION"

# 3. Tailor the resume externally (Claude Code, another agent, or manual edit),
#    then submit the full rewritten text
go-apply submit-tailored-resume --session "$SESSION" --file tailored.md

# 4. Persist the application record and close the session
go-apply finalize --session "$SESSION"
```

## Maintaining the tailor prompt

The `tailor_resume` MCP prompt embeds `internal/mcpserver/skills/resume-tailor.md` at build time via `//go:embed`. This file is vendored from the grimoire repository. To refresh it:

```bash
make sync-skill
```

By default `sync-skill` reads from `~/workplace/the-scriptorium/grimoire/skills/resume-tailor/SKILL.md`. Override with:

```bash
RESUME_TAILOR_SKILL_SRC=/path/to/SKILL.md make sync-skill
```

`sync-skill` atomically replaces the vendored file and regenerates the `.sha256` integrity sentinel. The test `TestTailorSkillBodyIntegrityHash` catches any mismatch between the embedded body and the sentinel at test time.

For contributors who do not have the grimoire: the vendored copy at `internal/mcpserver/skills/resume-tailor.md` is the authoritative source. Do not edit it by hand; submit a PR with the updated file and a re-generated `.sha256`.

## Getting a rendered PDF (external)

go-apply produces a tailored resume as **text**. To produce a rendered PDF, run the `resume-tailor` skill separately.

**Where the skill lives:** The typical path for maintainers is `~/workplace/the-scriptorium/grimoire/skills/resume-tailor/`. Other users install the skill per the skill's own README.

**What the skill needs from go-apply:**
- The `tailored_text` returned by `submit_tailored_resume`
- The extracted JD keywords (from `submit_keywords` output)
- Your resume template (`.tex` preferred; the skill's `template_generator.py` can produce one from an existing resume)

**Prerequisites the skill requires:**
- LaTeX: `tectonic` (preferred) or `xelatex` via MacTeX/TeXLive
- Python: `uv` (preferred) or `python3` + `pip`

Refer to the skill's own `setup.sh` for the authoritative install steps. go-apply does not install these for you.

**Minimal bridging:** After running the go-apply flow, open a Claude Code session in a directory that has the skill loaded, paste the tailored text, and ask "render this as a PDF using resume-tailor" — the skill's manual-trigger mode takes it from there. Precise scripting is the skill's own documentation territory.

## Migrating from v0.2.x

**If you were using the TUI to tailor resumes:** The TUI is removed. Install Claude Code and use MCP mode (primary path), or chain the CLI subcommands documented in "Scripted usage."

**If you were calling `submit_tailor_t1` and `submit_tailor_t2`:** These are replaced by a single `submit_tailored_resume`. Send the full rewritten resume text in one call.

**If you relied on the TUI's progress UI:** There is no equivalent. Run in MCP mode (Claude Code shows per-tool progress) or use the CLI with debug logging enabled (`go-apply config set log_level debug`).

**If you relied on the `job_application_workflow` MCP prompt:** It is replaced by `tailor_resume`. Fetch the new prompt name.

**If you got PDFs directly from go-apply:** See "Getting a rendered PDF (external)" above. The `resume-tailor` skill produces them; run it externally after the go-apply flow.

## CLI Reference

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

### `go-apply load-jd`

Fetch or accept a job description and start a session.

| Flag | Default | Description |
|------|---------|-------------|
| `--url <url>` | — | URL of the job posting to fetch |
| `--text <jd>` | — | Raw job description text (mutually exclusive with `--url`) |

### `go-apply score`

Score resumes against extracted JD keywords.

| Flag | Default | Description |
|------|---------|-------------|
| `--session <id>` | — | Session ID from `load-jd` (required) |

### `go-apply submit-tailored-resume`

Submit a tailored resume and rescore.

| Flag | Default | Description |
|------|---------|-------------|
| `--session <id>` | — | Session ID from `load-jd` (required) |
| `--file <path>` | — | Path to the tailored resume file (required) |
| `--changelog <json>` | — | JSON array of changelog entries (optional) |

### `go-apply finalize`

Persist the application record and close the session.

| Flag | Default | Description |
|------|---------|-------------|
| `--session <id>` | — | Session ID from `load-jd` (required) |
| `--cover-letter <path>` | — | Path to a cover letter file (optional) |

### `go-apply config`

Manage go-apply configuration. Subcommands:

| Subcommand | Usage | Description |
|------------|-------|-------------|
| `set` | `go-apply config set <key> <value>` | Set a config field by dot-notation key |
| `get` | `go-apply config get <key>` | Get a config field value by dot-notation key |
| `show` | `go-apply config show` | Show all config fields |

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

### `go-apply version`

Print the go-apply version. No flags.

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| `session_locked` | Another `go-apply` process is holding the session advisory lock | Wait for the other process to finish, or check `ps` for a stuck process — the lock releases on process exit |
| `rescore_failed` | Scoring crashed internally | The envelope says "see server logs for details"; check stderr / `slog` output for the structured record with `error` attribute |
| `invalid_changelog` | Changelog entry failed validation | The error message names the failing field and value; common cases: unknown `action`, `reason` > 512 bytes, `keyword` > 128 bytes |
| `invalid_state` | Session is in the wrong phase for this command | The error lists current state and legal transitions |
| Agent won't stop trying to render a PDF | Prelude regression in the prompt | Run `go test ./internal/mcpserver/...` to verify the prompt assertions pass, and `make sync-skill` if the embedded body is out of sync |
| MCP prompt fetch returns a mismatched-hash error | `resume-tailor.md` changed without regenerating the `.sha256` sentinel | Re-run `make sync-skill` and commit both files |

## Roadmap

| Feature | Status |
|---------|--------|
| Multi-resume scoring with full breakdown | Shipped |
| MCP integration (Claude Code, Hermes, Openclaw) | Shipped |
| Agent-driven tailoring via `tailor_resume` prompt | Shipped |
| Headless CLI subcommand flow | Shipped |
