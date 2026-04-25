# Quickstart: Verify Tier 4 Schema Sections End-to-End

This guide verifies all three user stories from spec.md after implementation.

---

## Prerequisites

- `go-apply` MCP server running and connected to Claude Code
- A test resume YAML or text file with Tier 4 sections (see sample below)

---

## Step 1: Prepare a test resume with Tier 4 sections

Create a plain-text resume that includes Tier 4 sections. Example fragment:

```
LANGUAGES
Go (Fluent), Python (Proficient), Rust (Learning)

SPEAKING ENGAGEMENTS
GopherCon 2024 — "Building ATS-aware Resume Tools" — gophercon.com/talks/2024/ats
Bay Area Go Meetup 2023 — "Hexagonal Architecture in Go"

OPEN SOURCE
go-apply — Maintainer — github.com/user/go-apply — ATS-aware resume tailoring CLI
pdfscalpel — Contributor — github.com/user/pdfscalpel — PDF text extraction

PATENTS
"Method for ATS-aware resume rendering" — US12345678 — 2024-01-15

INTERESTS
Distributed systems, compiler design, trail running

REFERENCES
Available upon request
```

---

## Step 2: Onboard the resume

```
mcp__go-apply__add_resume with label="tier4-test" and the resume text above
```

Expected: tool returns success; no error about unknown sections.

---

## Step 3: Run ATS preview

```
mcp__go-apply__preview_ats_extraction with label="tier4-test"
```

Expected response:
- `extracted_text` contains all Tier 4 section headings and their content
- No Tier 4 section is silently absent from the extraction
- Existing sections (experience, skills, etc.) remain unaffected

---

## Step 4: Verify render (no Tier 4 sections — regression check)

Onboard a resume with ONLY existing sections (no Tier 4). Run `preview_ats_extraction`.

Expected: output identical to pre-spec behaviour. SC-006 — no regression.

---

## Step 5: Run tests

```bash
go test -race ./internal/model/... ./internal/service/render/... ./internal/service/extract/... ./internal/mcpserver/...
```

Expected: all green, including:
- JSON round-trip tests for each new entry struct
- Registry render order test (matches pre-refactor golden output for existing sections)
- `parseSectionsArg` acceptance of new keys
- `Extract([]byte)` stub identity test

---

## Step 6: Build and vet

```bash
go build ./... && go vet ./...
```

Expected: clean — no type errors from the `Extract([]byte)` signature change.

---

## Acceptance checklist

- [ ] US1: Tier 4 sections survive onboard → render → extract pipeline  
- [ ] US2: `render.Service.Render` dispatch loop contains no hardcoded section-name strings  
- [ ] US3: `port.Extractor.Extract` signature is `(data []byte) (string, error)`  
- [ ] SC-006: render output for existing-only resume is byte-for-byte identical before/after registry refactor  
- [ ] All tests green with `-race`  
- [ ] `go build ./... && go vet ./...` clean  
