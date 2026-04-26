# Research: Per-Tier Real-World ATS Scoring

## Decision 1 — PDF extraction library

**Decision**: `github.com/ledongthuc/pdf` (pure-Go)
**Rationale**: Already in `go.mod`. No OS subprocess, no install requirement, no 30s
timeout needed. The spike confirmed byte-for-byte parity with `pdftotext` on real resumes.
**Alternatives considered**: `pdftotext` (poppler-utils subprocess) — rejected because it
requires an OS binary, which may not be present, and adds subprocess complexity.

## Decision 2 — Latin-1 fix approach

**Decision**: Transliterate before render (mutate a copy of sections in `pdfrender.go`).
**Rationale**: `fpdf` core fonts (Arial) are Windows-1252 encoded. Characters outside
Latin-1 (> U+00FF) are silently dropped. Transliterating to closest ASCII before render
produces correct output without changing the font stack or upgrading fpdf.
**Mapping**: em-dash → `-`, en-dash → `-`, smart quotes → straight quotes, ellipsis →
`...`, bullet `•` → `-`, accented Latin letters → ASCII base (é→e, ü→u, ñ→n, etc.).
Characters with no ASCII equivalent get `?` and a `slog.Warn`.
**Alternatives considered**: Upgrade to TTF font — rejected: large scope change, font
embedding increases binary size, out of scope for this spec.

## Decision 3 — scoreSectionsPDF location

**Decision**: `internal/mcpserver/score_pdf.go` (package `mcpserver`)
**Rationale**: T0/T1/T2 handlers all live in `mcpserver`. Putting the helper here avoids
cross-package reach and keeps the deps injection pattern consistent with existing handlers.
`pipeline.ApplyConfig` already carries `PDFRenderer`, `Extractor`, and `Scorer` — the
helper uses those directly.
**Alternatives considered**: `internal/service/pipeline/` — rejected: pipeline is for
the headless CLI flow; MCP-specific scoring helpers don't belong there.

## Decision 4 — T0 concurrency model

**Decision**: `errgroup.WithContext` (from `golang.org/x/sync`) + goroutine per resume;
results collected in pre-allocated slice; hard error if any resume fails extraction.
**Rationale**: Spec FR-004 requires bounded latency and hard-fail-on-any-error. WaitGroup
alone cannot cancel in-flight goroutines on first failure — they run to completion and
emit post-failure logs. `errgroup.WithContext` propagates cancellation automatically,
satisfies both latency and no-silent-failures requirements, and returns the first error
cleanly. `golang.org/x/sync` is an indirect dependency in virtually every Go project.
**Alternatives considered**: `sync.WaitGroup` — rejected because it leaves goroutines
running after the first failure, violating the spirit of the no-silent-failures principle.

## Decision 5 — `scoreSectionsPDF` placement

**Decision**: `internal/mcpserver/score_pdf.go` for this spec; acknowledged as a
pre-placement risk for spec 010.
**Rationale**: All T0/T1/T2 handlers live in `mcpserver`. Hexagonal invariants are
respected (function depends only on port interfaces). Placing it here keeps the scope
minimal for 009.
**Risk**: Spec 010 introduces a session cache that implies reusing this function outside
MCP transport. If that happens, moving it to `internal/service/scoring/` will be required
to avoid duplication. The 010 plan MUST explicitly address this placement decision.
**Alternatives considered**: `internal/service/scoring/` now — valid but premature; adds
a new package for one function before the reuse need is confirmed.

## Decision 6 — Threshold recalibration

**Decision**: No code change needed. Calibration completed pre-implementation.
**Evidence**: Spike ran against 12 real cached JDs in `~/.local/share/go-apply/jd_cache/`.
Mean delta: +0.87 pts (PDF > plain-text). Threshold 70.0 unchanged — delta is below 1 pt
and shifts no routing decision. FR-010/FR-011 are closed.
**JD corpus**: The 12 JDs used for calibration are whichever files exist under
`jd_cache/` as of 2026-04-26. The implementer MUST list their filenames here during the
T015 regression test setup so SC-004 is independently reproducible.

## Resolved Unknowns

| Unknown | Resolution |
|---------|-----------|
| Which ledongthuc/pdf API to use? | `pdf.Open(path)` requires a file; use `pdf.NewReader(r, size)` with `bytes.NewReader(data)` to avoid temp files |
| Does ledongthuc/pdf return `""` on empty PDF? | `GetPlainText()` returns `(io.Reader, error)`; call `io.ReadAll(r)` to materialise `[]byte`, then `string(b)`. Empty PDF produces empty bytes — `len(text)==0` treated as error per FR-004 |
| Where does transliteration run? | At the top of `RenderPDF()`, before any fpdf calls — clean input guarantee for the whole render path |
