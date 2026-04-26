# Data Model: Honest Scoring Loop

## New Entities

### KeywordSurvival (`internal/model/survival.go`)

The result of comparing JD keywords against PDF-extracted text.

```go
type KeywordSurvival struct {
    Dropped         []string `json:"dropped"`
    Matched         []string `json:"matched"`
    TotalJDKeywords int      `json:"total_jd_keywords"`
}
```

**Rules**:
- `Dropped + Matched` must equal `TotalJDKeywords` (no keyword is in both lists)
- All lists are non-nil (empty slice, not nil) when no keywords qualify
- When no JD is loaded (`TotalJDKeywords == 0`), `Dropped` and `Matched` are empty slices
- Keyword strings are stored as received from the scorer (preserving scorer's original casing and normalization) — matching is case-insensitive at diff time, not normalized on storage

---

## Modified Entities

### `previewData` (inline struct in `session_tools.go`)

Adds `KeywordSurvival` to the existing response shape.

```go
type previewData struct {
    Label           string                `json:"label"`
    ConstructedText string                `json:"constructed_text"`
    SectionsUsed    bool                  `json:"sections_used"`
    KeywordSurvival model.KeywordSurvival `json:"keyword_survival"`
}
```

**Rules**:
- `KeywordSurvival` is always present (not omitempty) — zero value (`{[], [], 0}`) when no JD keywords available
- `SectionsUsed` is always `true` in a success response — after FR-005b removes the raw-text fallback, a missing sidecar returns an error rather than setting this to `false`. This field is a candidate for removal in a follow-up cleanup; for now it signals that the honest pipeline was used.

---

## Existing Entities Used (unchanged)

### `model.KeywordResult` (`internal/model/score.go`)

Source of truth for JD keywords evaluated by the scorer. The survival service derives its keyword set from:

```
all_jd_keywords = ReqMatched ∪ ReqUnmatched ∪ PrefMatched ∪ PrefUnmatched
```

This is the complete set of JD keywords the scorer evaluated — no re-extraction needed.

### `model.SectionMap` (`internal/model/resume.go`)

Input to both `port.Renderer` (plain text, tailor pipeline) and `port.PDFRenderer` (PDF bytes, honest scoring loop). Unchanged.

### `port.Extractor` (`internal/port/extract.go`)

Signature already `Extract(data []byte) (string, error)` from Spec 007. The real implementation shells out to `pdftotext` on those bytes.

---

## New Port Interface

### `port.PDFRenderer` (`internal/port/pdfrender.go`)

```go
type PDFRenderer interface {
    RenderPDF(sections *model.SectionMap) ([]byte, error)
}
```

**Contract**:
- Returns valid PDF bytes or an error — never returns both non-nil
- Returns `nil, error` for nil input
- Bytes are never written to disk by the interface — in-memory only
- Must iterate sections via the registry (Spec 007) so new sections are included automatically

---

## Dependency Graph

```
model.SectionMap
    ↓
port.PDFRenderer.RenderPDF()    →    []byte (PDF, in-memory)
                                          ↓
                               port.Extractor.Extract()    →    string (extracted text)
                                                                      ↓
                                                     survival.Service.Diff(keywords, extractedText)
                                                                      ↓
                                                          model.KeywordSurvival
                                                                      ↓
                                                          previewData.KeywordSurvival
```
