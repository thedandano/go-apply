# Contract: go-apply doctor

## CLI Command

```
go-apply doctor
```

Subcommand of the root Cobra command. No flags required.

## Checks (in order)

| Check | Pass condition | Fail output | Exit |
|-------|---------------|-------------|------|
| `pdftotext` on PATH | `exec.LookPath("pdftotext") == nil` | `[MISSING] pdftotext — install poppler-utils (Linux) or: brew install poppler (macOS)` | 1 |

## Pass Output Format

```
[OK] pdftotext — /usr/bin/pdftotext
```

## Overall Exit Code

- `0` if all checks pass
- `1` if any check fails

## Test Injection Seam

The `doctor` command MUST accept an injectable `lookPath func(string) (string, error)` dependency (defaulting to `exec.LookPath`) so unit tests can simulate presence or absence of `pdftotext` without manipulating the system `PATH`:

```go
type DoctorCmd struct {
    lookPath func(string) (string, error) // defaults to exec.LookPath
}
```

## Integration with preview_ats_extraction

When `pdftotext` is not found by the extractor, the error message must include: `"run go-apply doctor to diagnose missing dependencies"`.
