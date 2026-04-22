# go-apply

AI-powered job application CLI. Scores your resume against job postings, tailors it, and generates cover letters.

[![CI](https://github.com/thedandano/go-apply/actions/workflows/ci.yml/badge.svg)](https://github.com/thedandano/go-apply/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/thedandano/go-apply)](https://github.com/thedandano/go-apply/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/thedandano/go-apply)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/thedandano/go-apply)](https://goreportcard.com/report/github.com/thedandano/go-apply)
[![Powered by Claude](https://img.shields.io/badge/Powered%20by-Claude-blueviolet?logo=anthropic)](https://anthropic.com)
[![Works with Claude Code](https://img.shields.io/badge/Works%20with-Claude%20Code-blueviolet?logo=anthropic)](https://claude.ai/code)
[![Works with OpenClaw](https://img.shields.io/badge/Works%20with-OpenClaw-orange)](https://github.com/openclaw)
[![Works with Hermes](https://img.shields.io/badge/Works%20with-Hermes-teal)](https://github.com/hermes-agent)

## Modes

| Mode | Command | Use case |
|------|---------|----------|
| MCP Server | `go-apply serve` | Claude Code and MCP-compatible agents — agent drives the full workflow |
| Headless CLI | `go-apply run --url <url>` | Agents without MCP support — consume JSON output |

MCP is the recommended path. The host agent orchestrates everything, including first-time onboarding. For agents without MCP, see [Headless (Non-MCP Agents)](#headless-non-mcp-agents).

## Quick Start (MCP)

1. **Install**
   ```bash
   curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | bash
   ```

2. **Register with your agent**
   ```bash
   go-apply setup mcp --agent claude    # Claude Code
   go-apply setup mcp --agent hermes    # Hermes
   go-apply setup mcp --agent openclaw  # Openclaw
   ```

3. **Tell your agent to apply**

   The agent checks your profile on first use and calls `onboard_user` automatically if you haven't onboarded yet:
   - *"Onboard my resume at ~/docs/resume.md"*
   - *"Score my resume against this job posting: https://..."*

## Headless (Non-MCP Agents)

For agents that call CLI tools directly and consume JSON output.

### Setup

1. **Configure the LLM** (required for headless mode):
   ```bash
   go-apply config set orchestrator.base_url https://api.anthropic.com/v1
   go-apply config set orchestrator.model claude-sonnet-4-6
   go-apply config set orchestrator.api_key sk-ant-...   # or set GO_APPLY_API_KEY env var
   ```

2. **Onboard your resume** (required before `run`):
   ```bash
   go-apply onboard --resume ~/docs/resume.md
   # Optionally add skills and accomplishments to improve tailoring:
   go-apply onboard --resume ~/docs/resume.md --skills ~/docs/skills.md --accomplishments ~/docs/accomplishments.md
   ```

3. **Run against a job posting**:
   ```bash
   go-apply run --url https://example.com/jobs/123
   # or paste raw JD text when the page is paywalled:
   go-apply run --text "We are hiring a senior Go engineer..."
   ```

### Output

JSON to stdout, pipeline events (step-started/completed/failed) as JSON lines to stderr:

```json
{
  "status": "success",
  "jd": { "title": "Senior Go Engineer", "company": "Acme", "required": ["go", "kubernetes"] },
  "scores": {
    "my-resume": {
      "breakdown": {
        "keyword_match": 40.5,
        "experience_fit": 18.0,
        "impact_evidence": 8.0,
        "ats_format": 9.0,
        "readability": 4.5
      }
    }
  },
  "best_score": 80.0,
  "best_resume": "my-resume",
  "cover_letter": { "text": "...", "channel": "COLD", "word_count": 180 }
}
```

## Installation

### Install script (recommended)

```bash
curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | bash
```

Options:
```bash
# Specific version
curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | VERSION=0.2.0 bash

# Custom install directory (system-wide)
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
# Required for headless/CLI mode only — not needed for MCP (the agent is the orchestrator)
orchestrator:
  base_url: https://api.anthropic.com/v1
  model: claude-sonnet-4-6
  api_key: sk-ant-...        # or set GO_APPLY_API_KEY env var

# Used in cover letters — set once, applies to MCP and CLI
user_name: "Your Name"
occupation: "Software Engineer"
location: "San Francisco, CA"
linkedin_url: "https://linkedin.com/in/yourprofile"
years_of_experience: 7
default_seniority: senior
```

All tunable scoring constants (weights, thresholds, limits) live in `internal/config/defaults.json`.

## Commands

### `go-apply onboard`

Store resumes, skills, and accomplishments in the data directory. Required before `go-apply run`.

```bash
go-apply onboard --resume ~/docs/resume.md
go-apply onboard --resume ~/docs/resume.md --skills ~/docs/skills.md --accomplishments ~/docs/accomplishments.md

# Reset all stored docs
go-apply onboard --reset
go-apply onboard --reset --yes   # skip confirmation (non-interactive)
```

| Flag | Description |
|------|-------------|
| `--resume <path>` | Resume file (repeatable; at least one required) |
| `--skills <path>` | Skills reference file (optional — improves T1 keyword targeting) |
| `--accomplishments <path>` | Accomplishments file (optional — improves T2 bullet rewriting) |
| `--reset` | Delete all stored resumes, skills, and accomplishments |
| `--yes` | Skip confirmation prompt for `--reset` |

### `go-apply run`

Run the full apply pipeline against a job description. Requires prior `go-apply onboard`.

```bash
go-apply run --url https://example.com/jobs/123
go-apply run --text "We are hiring..."
go-apply run --url <url> --channel REFERRAL
go-apply run --url <url> --accomplishments ~/docs/accomplishments.md
```

| Flag | Default | Description |
|------|---------|-------------|
| `--url <url>` | — | URL of the job posting to fetch |
| `--text <jd>` | — | Raw JD text (mutually exclusive with `--url`) |
| `--channel <channel>` | `COLD` | Application channel: `COLD`, `REFERRAL`, `RECRUITER` |
| `--accomplishments <path>` | — | Accomplishments doc for T2 bullet rewriting (optional) |

### `go-apply serve`

Start the MCP stdio server. No flags. Registered automatically by `go-apply setup mcp` — you don't typically call this directly.

### `go-apply setup mcp`

Register or unregister go-apply as an MCP server in an AI agent's config.

```bash
go-apply setup mcp --agent claude     # Claude Code
go-apply setup mcp --agent openclaw
go-apply setup mcp --agent hermes
go-apply setup mcp --agent all        # all known agents at once

go-apply setup mcp --agent claude --remove    # unregister
go-apply setup mcp --agent claude --override  # overwrite existing registration
```

The command is idempotent — running it again without `--override` reports "already registered" and makes no changes. On a TTY you will be prompted to confirm any overwrite.

### `go-apply config`

Manage configuration without editing the file directly.

| Subcommand | Usage | Description |
|------------|-------|-------------|
| `set` | `go-apply config set <key> <value>` | Set a config field |
| `get` | `go-apply config get <key>` | Get a config field value |
| `show` | `go-apply config show` | Show all fields (API keys redacted) |

Valid keys: `orchestrator.base_url`, `orchestrator.model`, `orchestrator.api_key`, `user_name`, `occupation`, `location`, `linkedin_url`, `years_of_experience`, `default_seniority`, `log_level`, `verbose`.

### `go-apply logs`

View recent log entries.

```bash
go-apply logs              # last 100 lines
go-apply logs -n 200       # last 200 lines
go-apply logs -f           # follow (tail -f)
tail -f ~/.local/state/go-apply/logs/go-apply-$(date +%Y-%m-%d).log | grep ERROR
```

| Flag | Shorthand | Default | Description |
|------|-----------|---------|-------------|
| `--lines <n>` | `-n` | `100` | Number of recent lines to show |
| `--follow` | `-f` | `false` | Watch for new log lines (tail -f mode) |

### `go-apply update`

Update go-apply to the latest GitHub release. No flags.

### `go-apply version`

Print the current version. No flags.

## Logging

Logs are written to `~/.local/state/go-apply/logs/go-apply-YYYY-MM-DD.log` — one file per day, last 50 retained.

```bash
go-apply config set log_level debug   # debug | info | warn | error (default: info)
go-apply config set verbose true      # true = full request/response payloads; false = truncated at 2 KB
```

MCP debug logging: set `log_level` once via `go-apply config set log_level debug` — both CLI and MCP server read it on startup.

## MCP Tools

Reference for agent authors. Exposed by `go-apply serve`:

| Tool | Purpose |
|------|---------|
| `onboard_user` | Store resume, skills, and accomplishments |
| `add_resume` | Add or replace a single resume |
| `get_config` | Read all config fields (API keys redacted) |
| `update_config` | Set a config field by dot-notation key |
| `load_jd` | Fetch JD by URL or raw text; returns `session_id` + `jd_text` |
| `submit_keywords` | Score resumes against extracted keywords; returns scores + `next_action` |
| `submit_tailor_t1` | Inject missing keywords into the Skills section; rescores |
| `submit_tailor_t2` | Rewrite Experience bullets to surface missing keywords; rescores |
| `finalize` | Persist the application record and close the session |

### Score thresholds

| Score | `next_action` | Meaning |
|-------|--------------|---------|
| ≥ 70 | `cover_letter` | Strong fit — proceed to cover letter |
| 40–69 | `tailor_t1` | Moderate fit — tailoring recommended |
| < 40 | `advise_skip` | Structural mismatch — tailoring won't close the gap |

The `job_application_workflow` MCP prompt contains full orchestration instructions for the host agent.
