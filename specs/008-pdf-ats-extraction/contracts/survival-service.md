# Contract: survival.Service

## Package

`internal/service/survival/`

## Interface (internal — not a port)

```go
// Diff computes which keywords from the JD survived the render→extract pipeline.
// keywords: all JD keywords (req + pref, matched + unmatched) from model.KeywordResult
// extractedText: plain text returned by port.Extractor
func (s *Service) Diff(keywords []string, extractedText string) model.KeywordSurvival
```

## Contract

| Condition | Behaviour |
|-----------|-----------|
| `keywords` is empty | Return `{Dropped:[], Matched:[], TotalJDKeywords:0}` |
| `extractedText` is empty | All keywords land in `Dropped`; `TotalJDKeywords = len(keywords)` |
| Keyword present in extracted text (case-insensitive whole-word match) | Keyword in `Matched` |
| Keyword absent | Keyword in `Dropped` |
| `len(Dropped) + len(Matched)` | Always equals `TotalJDKeywords` |
| Return values | All slices non-nil (empty slice, never nil) |

## Keyword Set Derivation

The caller (`HandlePreviewATSExtractionWithConfig`) computes the full keyword list **and deduplicates it** before passing it to `Diff`:

```go
raw := append(
    append(sess.ScoreResult.Keywords.ReqMatched, sess.ScoreResult.Keywords.ReqUnmatched...),
    append(sess.ScoreResult.Keywords.PrefMatched, sess.ScoreResult.Keywords.PrefUnmatched...)...,
)
// Deduplicate (same keyword may appear in both required and preferred lists).
seen := make(map[string]struct{}, len(raw))
keywords := raw[:0]
for _, kw := range raw {
    if _, ok := seen[kw]; !ok {
        seen[kw] = struct{}{}
        keywords = append(keywords, kw)
    }
}
```

**Deduplication is the caller's responsibility.** `TotalJDKeywords` equals `len(unique keywords)`. The survival service receives a deduplicated list and the `len(Dropped) + len(Matched) == TotalJDKeywords` invariant is guaranteed to hold.

## Pattern Matching

Same strategy as the scorer: case-insensitive whole-word regexp per keyword. The survival service duplicates this logic rather than importing it from the scorer (decoupling by duplication — the logic is trivial).

## No Error Return

`Diff` returns `model.KeywordSurvival` (not an error). All inputs are already validated by the caller. If matching panics, that is a bug — not a recoverable condition.
