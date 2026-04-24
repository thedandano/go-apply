# Research: T1 Skill Section Rewrites

All decisions resolved from codebase exploration. No external research required.

## Decision Log

**Decision**: Reuse `port.BulletRewrite` (`{Original, Replacement}`) for `skill_rewrites`
**Rationale**: T1 and T2 now share the same input shape; no new type needed; orchestrator
already knows the shape from T2 experience.
**Alternatives considered**: New `SkillRewrite` type with `replace`/`with` field names —
rejected because it duplicates an existing type and diverges the T1/T2 contracts for no gain.

---

**Decision**: `ApplySkillsRewrites` uses `strings.ReplaceAll` scoped to the Skills section
**Rationale**: Mirrors `ApplyBulletRewrites` exactly; replace-all within a bounded section
is safe and simpler than first-match logic; duplicate exact strings in a Skills section are
a degenerate case that does not warrant added complexity.
**Alternatives considered**: First-match only — rejected; requires a different implementation
pattern and provides no practical benefit.

---

**Decision**: `ExtractSkillsSection` returns `(section string, start, end int, found bool)`
**Rationale**: Returning indices allows `ApplySkillsRewrites` to splice the modified section
back without re-scanning the full resume. Cleaner than returning the section alone and
reconstructing via string search.
**Alternatives considered**: Return section string only + reconstruct via `strings.Replace` on
full text — rejected; risks replacing text outside the section if the section content also
appears elsewhere in the resume.

---

**Decision**: `submit_keywords` loads best resume text on-demand to extract `skills_section`
**Rationale**: Simplest change; `loadBestResumeText` helper already exists and is used by T1/T2
handlers; no refactor of `ScoreResumes` required.
**Alternatives considered**: Pre-compute during `ScoreResumes` — deferred; would require
threading the extracted section through the scoring pipeline, higher blast radius.
