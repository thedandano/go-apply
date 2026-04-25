# Contract: `port.Extractor` Interface (updated)

## Interface (`internal/port/extract.go`)

```go
// Extractor extracts plain text from document bytes for ATS analysis.
// Implementations must be deterministic for the same input bytes.
type Extractor interface {
    Extract(data []byte) (string, error)
}
```

## Change from previous signature

| Version | Signature |
|---|---|
| Before (spec 004 stub) | `Extract(text string) (string, error)` |
| After (this spec) | `Extract(data []byte) (string, error)` |

**Why**: A real PDF extractor receives raw binary. Converting PDF bytes to a Go `string` before passing to `Extract` would corrupt multi-byte sequences. The `[]byte` signature is the correct contract for the eventual implementation.

## Stub implementation (`internal/service/extract/extract.go`)

The identity stub converts bytes to string and returns unchanged:

```go
func (s *Service) Extract(data []byte) (string, error) {
    return string(data), nil
}
```

This is the correct stub behaviour: whatever text was rendered is passed through as-is. Spec C replaces this with a real `pdftotext`-backed implementation.

## Call-site updates (`internal/mcpserver/session_tools.go`)

Two call sites must be updated:

```go
// line ~717 (preview_ats_extraction)
extracted, err := extractSvc.Extract([]byte(rendered))

// line ~735 (preview_ats_extraction, rawText path)
extracted, err := extractSvc.Extract([]byte(rawText))
```

The `[]byte(s)` cast is explicit and intentional — it signals "this is the pre-Spec-C stub path; Spec C will pass real PDF bytes here instead."

## Invariants

1. **No silent conversion**: callers must not silently degrade by passing partial input. If the rendered string is empty, pass `[]byte("")` — do not skip the call.
2. **Error is explicit**: callers must check `err` and return it wrapped with context per constitution IV.
3. **Stub is pure**: the identity stub has no side effects and is safe to call in tests without any setup.
