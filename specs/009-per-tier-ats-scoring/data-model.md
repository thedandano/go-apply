# Data Model: Per-Tier Real-World ATS Scoring

No new persistent entities. All changes are to existing types or ephemeral response structs.

## Changed: `extract.Service` internals

**File**: `internal/service/extract/extract.go`

Before: wraps `pdftotext` OS subprocess with a 30s timeout.
After: calls `ledongthuc/pdf` in-process. Signature unchanged — `port.Extractor` interface
is unaffected. Callers require no changes.

```go
// port.Extractor — unchanged
type Extractor interface {
    Extract(ctx context.Context, data []byte) (string, error)
}
```

## New function: `transliterateLatin1`

**File**: `internal/service/pdfrender/latin1.go`

```go
// transliterateLatin1 returns a deep copy of sections with all string fields sanitized
// so every character falls within Latin-1 (≤ U+00FF). Characters with a known ASCII
// equivalent are replaced; others are replaced with '?' and logged at warn.
// The original sections pointer is never mutated. Callers MUST NOT persist the returned
// copy as the resume sidecar — it is for rendering only.
func transliterateLatin1(sections *model.SectionMap) model.SectionMap
```

Mapping table (minimum per FR-002):

| Input | Output |
|-------|--------|
| U+2014 em-dash | `-` |
| U+2013 en-dash | `-` |
| U+2022 bullet | `-` |
| U+2018 left single quote | `'` |
| U+2019 right single quote | `'` |
| U+201C left double quote | `"` |
| U+201D right double quote | `"` |
| U+2026 ellipsis | `...` |
| U+00A0 non-breaking space | ` ` (regular space) |
| U+2010 hyphen | `-` |
| U+2011 non-breaking hyphen | `-` |
| U+00C0–U+00FF accented Latin | ASCII base letter |
| anything else > U+00FF | `?` + slog.Warn (codepoint + field name only — NO surrounding text) |

## New function: `scoreSectionsPDF`

**File**: `internal/mcpserver/score_pdf.go`

```go
// scoreSectionsPDF sanitizes sections, renders to PDF, extracts text via ledongthuc/pdf,
// and scores the extracted text against jd. Returns a hard error on any failure,
// including empty extracted text (len==0 is treated as extraction failure here —
// not in the adapter — so the error message names the resume label).
// Callers set ScoringMethod = ScoringMethodPDFExtracted on the response after this call.
func scoreSectionsPDF(
    ctx context.Context,
    sections *model.SectionMap,
    label string,
    sessionID string,
    jd *model.JDData,
    cfg *config.Config,
    deps *pipeline.ApplyConfig,
) (model.ScoreResult, error)
```

Logs emitted (structured, per Constitution V — session_id is the correlation identifier):
- `slog.InfoContext(ctx, "score_pdf.render", "session_id", sessionID, "label", label, "sections_count", n)`
- `slog.InfoContext(ctx, "score_pdf.extract", "session_id", sessionID, "label", label, "extracted_bytes", n)`
- `slog.InfoContext(ctx, "score_pdf.score", "session_id", sessionID, "label", label, "score_total", total)`
- `slog.ErrorContext(ctx, "score_pdf.failed", "session_id", sessionID, "label", label, "stage", stage, "error", err)`

**Empty-text detection**: `scoreSectionsPDF` checks `len(text) == 0` after extraction and
returns an error with the resume label. The adapter (`extract.Service`) returns `("", nil)`
for a zero-byte output — the caller is responsible for treating that as a failure.

## Changed: tier response structs (T0 / T1 / T2)

**File**: `internal/mcpserver/session_tools.go`

`ScoringMethod string` added to each anonymous response struct, populated from the
package-level constant (not a string literal):

```go
// Defined once in internal/mcpserver/score_pdf.go
const ScoringMethodPDFExtracted = "pdf_extracted"

// T0 response (submitKeywordsData)
type submitKeywordsData struct {
    ...
    ScoringMethod string `json:"scoring_method"`
}
// set as: resultData.ScoringMethod = ScoringMethodPDFExtracted

// T1 response (t1Data) — same pattern
// T2 response (t2Data) — same pattern
```

## Unchanged

- `port.PDFRenderer` — interface unchanged
- `port.Extractor` — interface unchanged
- `port.SurvivalDiffer` — interface unchanged
- `port.Scorer` — interface unchanged
- `model.ScoreResult` — unchanged
- `model.KeywordSurvival` — unchanged (used by preview_ats_extraction, not this spec)
- `Session` struct — unchanged
- `pipeline.ApplyConfig` — unchanged (already carries PDFRenderer, Extractor, SurvivalDiffer)
