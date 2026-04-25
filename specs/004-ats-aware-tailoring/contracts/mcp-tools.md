# MCP Tool Contracts — ATS-Aware Resume Tailoring

Wire format for every MCP tool affected by this feature. All responses use the existing envelope `{ session_id, next_action, data | error }` defined in `internal/mcpserver/envelope.go`.

Types referenced below:
- `SectionMap` — see `data-model.md` §SectionMap.
- `ScoreResult` — unchanged; see `internal/model/score.go`.
- `Edit`, `EditRejection` — see `data-model.md` §EditEnvelope.

`schema_version` is always `1` in this release.

---

## 1. `onboard_user` (modified)

**Inputs (new):**
| Field | Type | Required | Description |
|---|---|---|---|
| `resume_content` | string | yes | Raw resume text. |
| `resume_label` | string | yes | Filesystem-safe label. |
| `sections` | JSON object `SectionMap` | yes (MCP mode) | Parsed sections from the orchestrator. |
| `skills` | string | no | Unchanged. |
| `accomplishments` | string | no | Unchanged. |

**Validation order:**
1. `resume_content` non-empty.
2. `resume_label` non-empty, no path separators.
3. `sections` parses as `SectionMap` and passes `ValidateSectionMap` (required keys, known keys only, schema version = 1).

**Error responses (typed codes):**
- `missing_sections` — sections omitted in MCP mode.
- `invalid_sections` — JSON parse or schema validation failed. Body includes `SchemaError` field + reason list.
- `invalid_label` — existing error path.

**Success data:**
```json
{
  "stored": ["resume:<label>", "ref:skills", "accomplishments:0", ...],
  "summary": { "resumes_added": 1, "skills_count": 42, "accomplishments_count": 3, "total_chunks": N },
  "schema_version": 1
}
```

---

## 2. `add_resume` (modified)

Identical new `sections` field and validation rules as `onboard_user`. Existing behavior otherwise unchanged.

---

## 3. `submit_keywords` (modified)

**Inputs:** unchanged (`session_id`, `jd_json`).

**Pre-condition:** The best-scored resume has `Sections != nil`. If any resume in scope is sections-missing, the handler returns `sections_missing` with a `raw` field containing the raw text for orchestrator re-onboard.

**Success data (breaking changes from current shape):**
```json
{
  "extracted_keywords": { ... unchanged ... },
  "scores":        { "<label>": ScoreResult, ... },
  "best_resume":   "<label>",
  "best_score":    85.4,
  "sections":      SectionMap,          // NEW — best_resume's sections, full map
  "schema_version": 1                   // NEW
}
```

**Removed:**
- `skills_section` (string, was `omitempty`) — removed; superseded by `sections.skills`.

**Error responses (new):**
- `sections_missing`: `{ "raw": "<raw text>", "hint": "call add_resume with sections" }`.

---

## 4. `submit_tailor_t1` / `submit_tailor_t2` (modified — unified envelope)

Both tools now accept the identical input shape. `next_action` routing differs only in the returned value (T1→T2 or T1→cover_letter; T2→cover_letter).

**Inputs:**
| Field | Type | Required | Description |
|---|---|---|---|
| `session_id` | string | yes | — |
| `edits` | JSON array of `Edit` | yes | Minimum 1 entry; maximum `MaxTier1SkillRewrites` for T1 and `MaxTier2BulletRewrites` for T2 (existing config knobs). |

The old `skill_rewrites` / `bullet_rewrites` fields are **removed**. Every edit specifies its target section explicitly.

**Per-op validation (applied in handler; rejections populate `edits_rejected`):**

| Condition | Reason string |
|---|---|
| Unknown `section` | `"unknown section: <section>"` |
| Unknown `op` | `"unknown op: <op>"` |
| `section/op` combination invalid | `"op <op> not supported for section <section>"` |
| `target` missing when required | `"target required for op <op> on section <section>"` |
| `value` missing when required | `"value required for op <op> on section <section>"` |
| `target` references nonexistent skill/bullet | `"target not found: <target>"` |
| Experience bullet ID malformed | `"malformed bullet id: <target>"` |

Global validation errors (wrong type, empty array, cap exceeded) return the existing typed error codes — not per-edit rejection. Cap example: `too_many_edits` with `limit` field.

**Success data:**
```json
{
  "previous_score":  float,
  "new_score":       ScoreResult,
  "edits_applied":   [Edit, ...],      // echoed back in order applied
  "edits_rejected":  [EditRejection],  // empty [] if all succeeded
  "sections":        SectionMap,       // post-edit
  "schema_version":  1
}
```

**`next_action` logic** (unchanged total, new inputs):
- T1 handler: `NextActionAfterT1(new_score)` — `"cover_letter"` if ≥70 else `"tailor_t2"`.
- T2 handler: always `"cover_letter"`.

---

## 5. `preview_ats_extraction` (new tool)

**Inputs:**
| Field | Type | Required | Description |
|---|---|---|---|
| `session_id` | string | yes | Session must be at `stateScored` or later. |

**Success data:**
```json
{
  "extracted_text": "string — output of extract(render(session.sections))",
  "schema_version": 1
}
```

**Errors:**
- `missing_session` — existing.
- `invalid_state` — existing.
- `sections_missing` — when the active best resume has no sections.

**Behavior today:** identity — the extracted text equals the rendered text. Future (FR-D01/D02): renderer emits PDF, extractor returns real extraction result. Callers of this tool do not change.

---

## 6. `finalize` (unchanged API)

`finalize` persists the record and closes the session. The persisted `ApplicationRecord` gains no new fields in this feature; the rendered post-edit text is already the `tailored_text` that existing finalize paths persist.

---

## Error envelope (existing, referenced)

All error responses conform to:
```json
{
  "session_id":  "<id or empty>",
  "stage":       "<tool name>",
  "error_code":  "<typed code>",
  "error":       "<human message>",
  "retryable":   true | false,
  "data":        <optional context map>
}
```

New typed codes introduced by this feature:
- `missing_sections`
- `invalid_sections`
- `sections_missing`
- `too_many_edits`
- `invalid_edits`
- `sections_unsupported_schema`
