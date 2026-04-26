# Quickstart: Honest Scoring Loop

## Prerequisites

```bash
# Install pdftotext (Poppler)
# macOS:
brew install poppler
# Ubuntu/Debian:
sudo apt-get install poppler-utils

# Verify
go-apply doctor
# Expected: [OK] pdftotext — /usr/bin/pdftotext
```

## Scenario 1: Preview ATS Extraction (honest loop)

```
# In MCP / Claude Code session:
mcp__go-apply__load_jd          # load a job description
mcp__go-apply__add_resume       # add resume with structured sections
mcp__go-apply__onboard_user     # (if first run)
mcp__go-apply__submit_keywords  # score — computes KeywordResult
mcp__go-apply__preview_ats_extraction
```

**Expected response**:
```json
{
  "label": "resume-v2",
  "constructed_text": "Jane Doe\njane@example.com\n...",
  "sections_used": true,
  "keyword_survival": {
    "dropped": ["machine learning"],
    "matched": ["kubernetes", "distributed systems"],
    "total_jd_keywords": 3
  }
}
```

## Scenario 2: Missing pdftotext

```
mcp__go-apply__preview_ats_extraction
```

**Expected error response**:
```json
{
  "error": "pdftotext_unavailable",
  "message": "extractor: pdftotext not found — run go-apply doctor to diagnose missing dependencies",
  "retryable": false
}
```

## Scenario 3: go-apply doctor output

```bash
go-apply doctor
# [OK] pdftotext — /usr/bin/pdftotext

# On failure:
# [MISSING] pdftotext — install poppler-utils (Linux) or: brew install poppler (macOS)
# exit code 1
```

## Test Scenarios (TDD — write these first)

```bash
# US1: PDF render + extraction round-trip
go test ./internal/service/pdfrender/... -run TestRenderPDF_ATS_Safe_Layout
go test ./internal/service/extract/... -run TestExtract_WithPdftotext

# US2: Keyword survival diff
go test ./internal/service/survival/... -run TestDiff_AllKeywordsSurvive
go test ./internal/service/survival/... -run TestDiff_SomeKeywordsDropped

# US3: Doctor command
go test ./internal/cli/... -run TestDoctor_PdftotextPresent
go test ./internal/cli/... -run TestDoctor_PdftotextMissing

# Integration: end-to-end preview_ats_extraction
go test ./internal/mcpserver/... -run TestHandlePreviewATSExtraction_HonestLoop
```
