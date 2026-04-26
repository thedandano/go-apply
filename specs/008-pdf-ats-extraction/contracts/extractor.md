# Contract: port.Extractor (real implementation)

## Interface (unchanged from Spec 007)

```go
type Extractor interface {
    Extract(data []byte) (string, error)
}
```

## Real Implementation Contract (replaces identity stub)

Invocation: `pdftotext - -` with PDF bytes on stdin, extracted text on stdout.

| Condition | Behaviour |
|-----------|-----------|
| `data` is valid PDF bytes | Return extracted plain text; `pdftotext` exit 0 |
| `pdftotext` not on PATH | Return `error("extractor: pdftotext not found — run go-apply doctor")` |
| `pdftotext` exits non-zero | Return error wrapping stderr output |
| `pdftotext` returns non-UTF-8 | Return error with encoding detail |
| `data` is empty or nil | Return `"", nil` (no bytes to extract) — callers must treat empty extracted text as a signal to check renderer output |
| Context deadline exceeded | Return context error — subprocess killed |

## Subprocess Timeout

Default: 10 seconds via `context.WithTimeout`. Must be configurable via `extract.Service` options for test injection. On timeout, the subprocess is killed via context cancellation and a context deadline error is returned.

## Invocation Safety

- MUST use `exec.Command("pdftotext", "-", "-")` with args as literal strings — never `exec.Command("sh", "-c", ...)`. Shell interpolation of resume content is prohibited.
- PDF bytes are written to the subprocess stdin; extracted text is read from stdout. No temp files.
- If `pdftotext` exits with non-zero status, its stderr MUST be captured, length-capped to 256 bytes, and stripped of non-printable bytes before being wrapped into the returned error. Raw stderr is NOT forwarded (PII risk).

## Test Injection Seam

`extract.Service` MUST expose a `lookPath func(string) (string, error)` field (defaulting to `exec.LookPath`) so unit tests can substitute a fake implementation without requiring `pdftotext` to be installed:

```go
type Service struct {
    lookPath func(string) (string, error) // defaults to exec.LookPath
    timeout  time.Duration                // defaults to 10s
}
```

Tests that require the real binary MUST use a build tag `//go:build integration` and be excluded from the default `go test ./...` run.

## No Silent Fallbacks

The previous identity implementation returned `string(data)` on any input. The real implementation MUST NOT fall back to this behavior. If `pdftotext` is unavailable, callers receive an explicit error.

## Compile-time assertion (required)

```go
var _ port.Extractor = (*Service)(nil)
```
