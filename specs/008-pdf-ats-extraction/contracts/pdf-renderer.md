# Contract: port.PDFRenderer

## Interface

```go
// PDFRenderer converts a SectionMap into PDF bytes suitable for ATS extraction.
// The concrete implementation lives in internal/service/pdfrender/.
type PDFRenderer interface {
    RenderPDF(sections *model.SectionMap) ([]byte, error)
}
```

## Guarantees

| Condition | Behaviour |
|-----------|-----------|
| `sections == nil` | Return `nil, error("pdfrender: nil sections")` — never return empty bytes |
| All sections empty (only ContactInfo.Name) | Return minimal valid PDF (single page, name only) — never crash |
| All 16 section types populated | All sections appear in output, in registry order |
| `pdftotext` parses the returned bytes | All section headings and content items present in `port.Renderer` output appear in the extracted text, in registry order — reachability guarantee, not string equality (fpdf wraps lines; the plain-text renderer does not) |
| Dependency unavailable (PDF library panics) | Return error — never silently produce corrupt bytes |

## Content Safety Requirements

- All string inputs MUST be validated as valid UTF-8 before embedding. Invalid UTF-8 MUST return `error` rather than produce a corrupt PDF.
- String content MUST be passed through the PDF library's documented escaping API — raw string concatenation into PDF output streams is prohibited.
- The renderer MUST return `nil, error` when the PDF output is zero-length (indicates a library-level silent failure).
- Golden-file regeneration: tests MUST honour the `UPDATE_GOLDEN=1` environment variable to overwrite golden files rather than asserting against them.

## Non-Guarantees (explicitly out of scope)

- Multi-column layout: default is always single-column ATS-safe
- PDF/A compliance: not required
- Font embedding metadata: non-deterministic; tests must not golden PDF bytes
- Writing to disk: the interface contract forbids it — bytes are in-memory only

## Compile-time assertion (required in implementation file)

```go
var _ port.PDFRenderer = (*Service)(nil)
```
