# Retro: Task 5 — Document Loader (2026-04-11)

## What shipped
`internal/loader/` — unified document text extractor satisfying `port.DocumentLoader`.
Dispatcher routes by extension to: DOCX (stdlib zip+xml), PDF (ledongthuc/pdf), plain text/markdown (os.ReadFile).

## What went well
- Two independent reviewers caught the same critical bug before merge (no coordination)
- Fix was surgical — 4 lines in `extractXMLText`, no architectural change
- Full check suite (build + vet + lint + gosec + tests) ran clean after every fix

## Issues caught in review

### Critical: Missing paragraph separators in DOCX extraction
`extractXMLText` concatenated all `<w:t>` runs with no separator. "John Doe" and "Software Engineer" on separate lines fused into "John DoeSoftware Engineer", corrupting every downstream scoring and tailoring operation.

**Fix:** emit `\n` on `</w:p>` EndElement.

**Root cause:** Initial implementation only tracked `<w:t>` elements and ignored paragraph boundaries. The test used `strings.Contains` on individual words so the bug was invisible.

**Prevention:** Test DOCX extraction with multi-paragraph input and assert the full output string, not substrings.

### Important: Test gaps
- `DOCXExtractor.Load` zip path had zero tests (only the raw XML parser was tested)
- `PDFExtractor` had zero tests

**Fix:** Added `TestDOCXExtractorLoad`, `TestDOCXExtractorLoad_MissingDocumentXML`, `TestDOCXExtractorLoad_InvalidZip`, `TestPDFExtractorLoad_MissingFile` using in-memory zips via `archive/zip.NewWriter`.

### Minor: gosec G104 on unhandled `rc.Close()` error
Initial commit didn't handle the close error. CI security scan caught it.

**Fix:** Capture `closeErr` and return it if no earlier error occurred.

**Prevention:** Run `gosec ./...` locally as part of pre-commit checks (now in memory).

## CI failures this task
- Security scan failed once (G104 unhandled close error) — should have run gosec locally before pushing. Now locked in as mandatory pre-commit step.

## Process note
This was the first task run in fully autonomous mode (no human reviewer). Two AI reviewers in parallel worked well — both independently identified the paragraph separator bug with high confidence.
