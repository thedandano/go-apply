# Research: Honest Scoring Loop

## Decision 1: PDF Generation Library

**Decision**: `github.com/go-pdf/fpdf` (pure Go)

**Rationale**:
- Zero system dependencies — no Chrome, no Webkit, no binary install required on the renderer side
- ATS-safe single-column layout is fully achievable: left-to-right text flow, standard fonts, no tables or embedded images
- Active maintained fork of the widely-used `jung-kurt/gofpdf`
- Sufficient fidelity for the resume format go-apply generates (text-only, section headings, bullet lists)

**Alternatives considered**:
- `chromedp` (headless Chrome): Much higher layout fidelity but requires Chrome installed — creates a second system binary dependency alongside `pdftotext`, and makes hermetic testing harder (Chrome startup, non-deterministic rendering). Rejected: complexity vs. benefit ratio too high for ATS-safe single-column output.
- `pdfcpu`: PDF manipulation library, not generation. Cannot produce new PDFs from scratch. Rejected.
- `maroto`: Invoice/report-style templates; no plain-text resume model. Rejected.

## Decision 2: pdftotext Invocation Strategy

**Decision**: `exec.Command("pdftotext", "-", "-")` with stdin/stdout — PDF bytes piped in, extracted text piped out. No temp files.

**Rationale**:
- Satisfies the in-memory PII constraint from the spec clarification (PDF bytes never written to disk)
- `pdftotext` supports `-` as both input and output for stdin/stdout mode
- Standard subprocess timeout via `context.WithTimeout` (5s default, sufficient for single-resume extraction)
- Error on non-zero exit code — explicit, no silent degradation

**Alternatives considered**:
- Write to `/tmp` + read back: simpler API but violates PII constraint (resume content in temp files). Rejected.
- Go PDF parsing library (e.g., `pdfcontent`): Extraction quality far below `pdftotext` for ATS accuracy. The whole point is to match what an ATS does. Rejected.

## Decision 3: New Port Interface vs. Extending Existing

**Decision**: New `port.PDFRenderer` interface separate from `port.Renderer`.

**Rationale**:
- `port.Renderer` returns `string` and is used by the tailor pipeline (LLM needs plain text, not PDF bytes). Changing its return type breaks callers at `session_tools.go:460,599`.
- Hexagonal architecture principle: ports are narrow — one capability per interface. PDF generation and plain-text generation are different capabilities with different consumers.
- `port.PDFRenderer` is only wired into `preview_ats_extraction`, keeping the tailor pipeline unchanged.

**Interface**:
```go
type PDFRenderer interface {
    RenderPDF(sections *model.SectionMap) ([]byte, error)
}
```

**Alternatives considered**:
- Change `port.Renderer` to return `[]byte`: breaks 2 tailor callers, requires adding a second extract call for plain text in the tailor pipeline. Rejected.
- Add a `RenderPDF(sections)` method to the existing `render.Service`: violates single-responsibility; also the plain-text renderer and PDF renderer are separate adapters. Rejected.

## Decision 4: Survival Diff — Data Source for JD Keywords

**Decision**: Use `sess.ScoreResult.Keywords` (already computed `KeywordResult`) as the keyword source. All JD keywords = `ReqMatched + ReqUnmatched + PrefMatched + PrefUnmatched`.

**Rationale**:
- The scorer already extracted and classified all JD keywords for the best resume. No redundant extraction pass needed.
- The survival diff reuses the same set: "of the keywords the scorer evaluated, which survive the PDF render→extract pipeline?"
- `survival.Service` takes the full keyword list + extracted text, runs the same pattern-matching logic as the scorer, returns `KeywordSurvival`.

**Alternatives considered**:
- Re-extract keywords from the JD independently in the survival service: duplicate work, possible divergence from scorer's classification. Rejected.
- Expose scorer's `compileKeywordPattern` as a shared package function: acceptable but couples scorer to survival service. Decision: survival service duplicates the pattern-matching logic (it's trivial — case-insensitive whole-word regex) to stay decoupled.

## Decision 5: Survival Service Location

**Decision**: New package `internal/service/survival/`.

**Rationale**:
- The survival diff is a distinct service concern (compare two text corpora against a keyword list). It belongs in its own package, not bolted onto scorer or extract.
- Clean separation: `survival.Service` takes `(keywords []string, extractedText string) → KeywordSurvival`. No dependency on scorer internals.

## Decision 6: Removing Silent Fallbacks in preview_ats_extraction

**Decision**: When real implementations are wired, the render/extract fallbacks at `session_tools.go:714` and `session_tools.go:719` become hard errors. The `sections_used=false` path (no SectionMap sidecar) becomes an explicit error: "no structured resume data — load a resume with sections to use honest scoring."

**Rationale**:
- Constitution IV (No Silent Failures): fallbacks that silently return a different quality of result than requested are defects.
- The fallback was acceptable for the stub era (both paths produced identical output). With real implementations, a fallback to raw text gives a misleading score.
- If `pdftotext` is missing: return `pdftotext_unavailable` error code referencing `go-apply doctor`.
- If render fails: return `render_failed` error.
- If no sections sidecar: return `no_sections_data` error with a clear message.

## Decision 7: go-apply doctor Command

**Decision**: New `doctor` subcommand in `internal/cli/doctor.go`, registered with the root Cobra command.

**Checks in order**:
1. `pdftotext` on PATH — `exec.LookPath("pdftotext")` — fail with "install poppler-utils (Linux) or `brew install poppler` (macOS)"
2. (Future: PDF library health check if needed)

**Output format**: Plain text per check, prefixed with `[OK]` or `[MISSING]`. Exit code 1 if any check fails.

## Golden Test Strategy

**Decision**: Golden files contain extracted text strings, not PDF bytes.

**Rationale**: PDF bytes are non-deterministic (fonts embed timestamps, creator metadata varies). Extracted text is deterministic for a given SectionMap + fpdf version.

**Implementation**: `testdata/golden/` directory under `internal/service/pdfrender/`. Test renders a fixed SectionMap and diffs extracted text against the golden file. `UPDATE_GOLDEN=1 go test` regenerates.
