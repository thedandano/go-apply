# go-apply

MCP server that helps you land job interviews — honestly. Scores your resume against job postings, tailors it with a two-tier cascade, and builds a compiled profile of your skills and stories for your AI agent to use.

[![CI](https://github.com/thedandano/go-apply/actions/workflows/ci.yml/badge.svg)](https://github.com/thedandano/go-apply/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/thedandano/go-apply)](https://github.com/thedandano/go-apply/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/thedandano/go-apply)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/thedandano/go-apply)](https://goreportcard.com/report/github.com/thedandano/go-apply)
[![Powered by Claude](https://img.shields.io/badge/Powered%20by-Claude-blueviolet?logo=anthropic)](https://anthropic.com)
[![Works with Claude Code](https://img.shields.io/badge/Works%20with-Claude%20Code-blueviolet?logo=anthropic)](https://claude.ai/code)
[![Works with OpenClaw](https://img.shields.io/badge/Works%20with-OpenClaw-orange)](https://github.com/openclaw)
[![Works with Hermes](https://img.shields.io/badge/Works%20with-Hermes-teal)](https://github.com/hermes-agent)

## How it works

go-apply runs as an MCP server. Your AI agent (Claude Code, Hermes, Openclaw) is the orchestrator — it calls the tools, extracts keywords from job descriptions, tags your stories with skills, and drives the full workflow. go-apply provides the storage, scoring, and tailoring.

```
Agent (Claude / Hermes / Openclaw)
  │
  ├─ onboard_user        Store resume, skills, accomplishments
  ├─ add_resume          Add or replace a resume variant
  ├─ compile_profile     Assemble profile from agent-tagged skills + stories
  ├─ create_story        Save an SBI accomplishment story, trigger recompile
  │
  ├─ load_jd             Fetch or accept a job description
  ├─ submit_keywords     Score resumes against extracted keywords
  ├─ submit_tailor_t1    Keyword injection into skills section
  ├─ submit_tailor_t2    Bullet rewriting in experience section
  ├─ preview_ats_extraction  Show resume text as an ATS would see it
  └─ finalize            Persist application record
```

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

3. **Ask your agent to onboard and apply**
   - *"Onboard my resume at ~/docs/resume.md"*
   - *"Compile my profile from the accomplishments I just gave you"*
   - *"Score my resume against this job posting and tailor it"*

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
years_of_experience: 7
default_seniority: senior
user_name: "Your Name"
occupation: "Software Engineer"
location: "San Francisco, CA"
log_level: info    # debug | info | warn | error
verbose: false     # true = full payloads in logs
```

No API key is needed for MCP mode — the host agent is the LLM. Scoring constants (weights, thresholds) live in `internal/config/defaults.json`.

> **Migration from older versions**: `orchestrator.*` keys in `config.yaml` are no longer used; you can remove them. The headless `go-apply run` command was removed in v2.

## Logging

Log files are written to `~/.local/state/go-apply/logs/go-apply-YYYY-MM-DD.log` — one file per calendar day (multiple invocations append); last 50 files retained.

```bash
# View recent logs
go-apply logs

# Watch live (tail -f equivalent)
go-apply logs --follow

# Show last 200 lines
go-apply logs --lines 200
```

Log level and verbose mode:
```bash
go-apply config set log_level debug
go-apply config set verbose true
```

### MCP server debug logging

Set `log_level` in the config file, or pass it via the `env` block in your agent's settings. Example for Claude Code:

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

## Profile workflow

The profile is the source of truth for who you are. Build it once; your agent uses it on every job application.

### 1. Onboard

```
Agent calls: onboard_user(resume_content, resume_label, skills, accomplishments)
```

Stores your resume(s), skills reference, and accomplishments text. Returns `needs_compile: true`.

### 2. Compile profile

```
Agent calls: compile_profile(skills, stories)
```

The agent tags your accomplishments with skill labels before calling this tool. `compile_profile` assembles the compiled profile from the agent-provided `skills` array and `stories` array (each story has an `accomplishment` string and `tags`). Returns a rich diff: coverage gained, skills added/removed, orphaned skills (skills with no supporting story).

### 3. Create stories (for orphaned skills)

```
Agent calls: create_story(skill, story_type, job_title, situation, behavior, impact)
```

Saves an SBI (Situation-Behavior-Impact) story to `accomplishments.json`. Returns `needs_compile: true` — the agent calls `compile_profile` again to pick up the new story.

### Accomplishments storage

Accomplishments are stored in `~/.local/share/go-apply/accomplishments.json`:

```json
{
  "schema_version": "1",
  "onboard_text": "Raw text from onboarding...",
  "created_stories": [
    { "id": "0", "skill": "Go", "type": "technical", "text": "..." },
    { "id": "1", "skill": "Kubernetes", "type": "project", "text": "..." }
  ]
}
```

Stories get sequential integer IDs (`"0"`, `"1"`, …). The compiled profile at `~/.local/share/go-apply/profile-compiled.json` is derived from this file plus the skills list.

## Job application workflow

Once your profile is compiled, the agent can score and tailor any job posting:

```
load_jd → submit_keywords → submit_tailor_t1 → submit_tailor_t2 → finalize
                           ↘ preview_ats_extraction (any post-scored state)
```

The agent receives `extraction_protocol` in the `load_jd` response — it extracts keywords from the JD text itself and calls `submit_keywords` with structured JD data.

## CLI Reference

### `go-apply serve`

Start the MCP stdio server. No flags. This is the primary way to use go-apply.

### `go-apply onboard`

Store resumes, skills, and accomplishments in the profile database directly from the CLI.

| Flag | Default | Description |
|------|---------|-------------|
| `--resume <path>` | — | Path to a resume file (repeatable; at least one required) |
| `--skills <path>` | — | Path to skills reference file (optional) |
| `--accomplishments <path>` | — | Path to accomplishments file (optional) |
| `--reset` | `false` | Delete profile database: inputs/, skills.md, accomplishments.json |
| `--yes` | `false` | Skip confirmation prompt for `--reset` |

### `go-apply config`

Manage go-apply configuration.

| Subcommand | Usage | Description |
|------------|-------|-------------|
| `set` | `go-apply config set <key> <value>` | Set a config field by dot-notation key |
| `get` | `go-apply config get <key>` | Get a config field value |
| `show` | `go-apply config show` | Show all config fields (API keys redacted) |

### `go-apply setup mcp`

Register or unregister go-apply as an MCP server in an agent's config.

| Flag | Default | Description |
|------|---------|-------------|
| `--agent <name>` | — | Agent to configure: `claude`, `openclaw`, `hermes`, `all` (required) |
| `--remove` | `false` | Unregister go-apply |
| `--override` / `--force` | `false` | Overwrite an existing registration |

The command is idempotent — running it again without `--override` reports "already registered" and makes no changes.

### `go-apply logs`

View recent go-apply log entries.

| Flag | Shorthand | Default | Description |
|------|-----------|---------|-------------|
| `--lines <n>` | `-n` | `100` | Number of recent lines to show |
| `--follow` | `-f` | `false` | Watch for new log lines (tail -f mode) |

### `go-apply doctor`

Diagnose configuration and profile health. No flags.

### `go-apply update`

Update go-apply to the latest GitHub release. No flags.

### `go-apply version`

Print the go-apply version. No flags.

## MCP tools reference

| Tool | Requires onboarding | Description |
|------|--------------------|----|
| `onboard_user` | — | Store resume, skills, accomplishments |
| `add_resume` | — | Add or replace a single resume variant |
| `compile_profile` | — | Assemble compiled profile from agent-tagged skills + stories |
| `create_story` | — | Save an SBI story to accomplishments.json |
| `get_config` | — | Return all config fields (API keys redacted) + profile status |
| `update_config` | — | Set a config field by dot-notation key |
| `load_jd` | Yes | Fetch or accept a job description; returns extraction protocol |
| `submit_keywords` | Yes | Score resumes against extracted JD keywords |
| `submit_tailor_t1` | Yes | Keyword injection into the skills section |
| `submit_tailor_t2` | Yes | Experience bullet rewriting |
| `preview_ats_extraction` | Yes | Show resume as an ATS would see it |
| `finalize` | Yes | Persist application record and close session |

## Roadmap

| Feature | Status |
|---------|--------|
| Resume scoring with full keyword breakdown | Shipped |
| Two-tier tailoring (T1 keyword injection + T2 bullet rewriting) | Shipped |
| MCP integration (Claude Code, Hermes, Openclaw) | Shipped |
| Compiled profile: skill-story graph, orphan detection | Shipped |
| Host-driven profile compilation (agent tags skills, pure assembler) | Shipped |
| SBI story creation with job context | Shipped |
| ATS extraction preview | Shipped |
| PDF scoring via rendered text | Shipped |
