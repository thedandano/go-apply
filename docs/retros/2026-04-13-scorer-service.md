# Retro: Scorer Service (PR #22)

**Date:** 2026-04-13
**Branch:** feat/scorer-service → feat/scorer-bug-fixes
**PR:** thedandano/go-apply#22

---

## What shipped

- `internal/service/scorer/scorer.go` — deterministic, pure-Go implementation of `port.Scorer`
- 5 scoring dimensions: KeywordMatch (45), ExperienceFit (25), ImpactEvidence (10), ATSFormat (10), Readability (10)
- 25 TDD tests covering all dimensions, edge cases, and error paths
- `config.EmbeddedDefaults()` unified to call `LoadDefaults()` — `defaults.json` is now the single source of truth (no hardcoded duplicate struct)
- `filler_phrases` and `readability_penalty_per_filler` added to `defaults.json`
- `port.Scorer.Score()` signature changed to pointer receiver (`*model.ScorerInput`) after golangci-lint `hugeParam` finding on push

---

## What went well

- TDD caught two real logic bugs before the implementation was even complete: word-boundary matching preventing "Go" from matching inside "Django", and empty-preferred contributing 1.0 pct instead of 0.0
- The advisor's pre-implementation call identified three requirements (filler phrases to defaults, impact regex false-positive guard, empty ResumeText error) that would have been review comments otherwise
- golangci-lint on push enforced the pointer-receiver change cleanly — hook caught what tests didn't
- Three-Opus audit after implementation was highly effective: found bugs the tests didn't cover (years matching as metric bullets, ATSFormat substring false-positives, SeniorityMatch never wired in production)

---

## Bugs found in post-merge audit (being fixed in feat/scorer-bug-fixes)

### Unanimous (all 3 auditors)

1. **`\d{3,}` matches calendar years** — date ranges ("2019–2023") fire as metric bullets. ImpactEvidence inflated for virtually every resume.
2. **Unknown `SeniorityMatch` silently zeros 60% of experience score** — map lookup returns 0.0 for unset/unknown key. Production code never sets this field; tests hard-code it.
3. **ATSFormat uses substring match** — "5 years of experience" matches the "experience" section check. Nearly all resumes score 10/10 regardless of structure.

### Majority (2 of 3 auditors)

4. **Empty-preferred keyword list penalizes candidate** — max achievable score is 31.5/45 when JD has no preferred keywords.
5. **Version-line veto discards entire lines** — "40% reduction using Python 3.11" loses the 40% metric.
6. **Filler phrase detection is substring-based** — "networked on" matches "worked on", false penalties applied.

### Unique (1 auditor, confirmed real)

7. **`\b` word-boundary fails for C++, C#, .NET** — keywords ending or starting with non-word characters never match.

---

## Synonym matching decision

The vector DB handles synonym matching at **retrieval** correctly — embedding "JS" lands near "JavaScript" in vector space, so augmentation pulls the right chunks. The scorer's matching problem is addressed by normalizing keywords at JD extraction time (LLM prompt emits canonical expanded forms, not abbreviations). No hardcoded synonym dictionary needed; LLM training data handles any career domain.

---

## What to do differently next time

- Include a **property-based / invariant test**: `score.Total() <= 100` for arbitrary inputs. Boundary arithmetic bugs won't be caught by example-based tests.
- **Test with unset/zero-value fields** by default in base fixtures. `baseInput()` should intentionally leave `SeniorityMatch` empty to catch the silent-zero bug at TDD time.
- Wire **all `ScorerInput` fields** through the pipeline before writing scorer tests — if the field isn't wired, the test fixture is lying.
- Run a **three-Opus audit earlier** (after test-writing, before implementation) not after PR merge.
