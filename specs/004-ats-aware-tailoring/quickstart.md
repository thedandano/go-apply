# Quickstart — ATS-Aware Resume Tailoring (feature 004)

End-to-end verification for the post-implementation system. Replays the **PlayStation Data Platform** scenario that originally surfaced the bugs, plus the new capabilities.

**Preconditions**
- `go build ./...` succeeds.
- `go test ./...` passes (contract + integration + unit tiers).
- `go vet ./...` clean.
- Resume `inputs/default.txt` exists with a "Skills & Abilities" heading (not "Skills").

---

## 1. Parity regression (T1 silent-failure bug)

**Failing pre-feature behavior:**
- `submit_tailor_t1` returned `skills_section_found: false`, applied zero rewrites, resume untouched.

**Steps:**
```bash
# MCP mode
go-apply serve mcp
```
Then from the orchestrator:
```text
add_resume   resume_label="default" resume_content=<raw> sections={...}
load_jd      jd_url="<playstation JD>"
submit_keywords session_id=<id> jd_json=<extracted>
submit_tailor_t1 session_id=<id> edits=[
  {"section":"skills","op":"replace","target":"AWS","value":"AWS, GCP"},
  {"section":"skills","op":"add","value":"Apache Spark"}
]
```

**Expected:**
- `submit_tailor_t1.data.edits_applied` contains both edits (length 2).
- `submit_tailor_t1.data.edits_rejected` is `[]`.
- `submit_tailor_t1.data.sections.skills` reflects both mutations.
- `submit_tailor_t1.data.new_score.total` > `previous_score`.

**Fail signal:** non-zero `edits_rejected` or unchanged `sections.skills`.

---

## 2. Alias-aware scoring (PySpark ↔ Apache Spark)

**Scenario:** JD requires "Apache Spark"; resume lists "PySpark".

**Steps:**
1. Run `submit_keywords` with a `jd_json` whose `required` contains `"Apache Spark"`.
2. Inspect `scores.<label>.keywords.req_matched`.

**Expected:** `"Apache Spark"` appears in `req_matched`, even though the literal token in the resume is `PySpark`. `scores.<label>.breakdown.keyword` reflects the match.

**Additional alias checks** (same pattern):
- `PostgreSQL` ↔ `Postgres`
- `Kubernetes` ↔ `K8s`
- `JavaScript` ↔ `JS`
- `TypeScript` ↔ `TS`

**Fail signal:** any of the above in `req_unmatched`.

---

## 3. Non-standard heading (no regex dependency)

**Scenario:** A resume uses `## Technical Stack` in place of any `Skills*` heading.

**Steps:**
1. `add_resume` with the resume's raw text AND a `sections` object where `sections.skills.kind == "flat"` and `sections.skills.flat == "Go, Rust, Python"`.
2. `submit_tailor_t1` with an `add` op targeting section `"skills"`.

**Expected:**
- Edit applied (`edits_applied` length 1, `edits_rejected` empty).
- `sections.skills.flat` contains the new value.

**Why:** the old `skillsHeaderRe` is deleted; section lookup is by JSON key. No heading regex exists to succeed or fail.

---

## 4. Section-aware T2 bullet rewrites

**Scenario:** After T1, score still < 70; orchestrator rewrites specific experience bullets.

**Steps:**
1. Pull `sections.experience` from the prior `submit_tailor_t1.data.sections`.
2. Send T2 with a `replace` op against a bullet ID:
   ```json
   submit_tailor_t2 session_id=<id> edits=[
     {"section":"experience","op":"replace","target":"exp-0-b2",
      "value":"Architected real-time ingest on Apache Spark / PySpark processing 1B events/day"}
   ]
   ```

**Expected:**
- `edits_applied` length 1, `edits_rejected` empty.
- `sections.experience[0].bullets[2]` is the new text.
- `new_score.breakdown.impact` ≥ previous.

**Malformed-ID sanity check:** submit the same edit with `target: "exp-99-b0"` (out-of-range) and confirm it returns in `edits_rejected` with `reason="target not found: exp-99-b0"`, while a second valid edit in the same envelope still applies.

---

## 5. Canonical-label rendering

**Scenario:** Resume onboarded with Skills heading = "Technical Stack", Experience = "Work History".

**Steps:**
1. `add_resume` with those headings in the raw text and the canonical keys in the `sections` object.
2. Call `preview_ats_extraction session_id=<id>`.

**Expected:** the rendered output contains `## Skills` and `## Work Experience` — the canonical labels from research §R7, NOT the raw headings. The raw text is preserved only inside `resume.RawText` for audit, not rendered.

---

## 6. Tiered order by YoE

**Scenario A (≥3 YoE):** experience-forward order.
- Expected render order: `Contact → Summary → Experience → Skills → Education → …`

**Scenario B (<3 YoE):** education-forward order.
- Resume sections: `contact`, `education`, `experience` (< 3 years combined), `skills`.
- Expected render order: `Contact → Summary → Education → Experience → Skills → …`

**Scenario C (orchestrator override):** `sections.order = ["contact","skills","experience","education"]`.
- Expected: rendered in exactly that order regardless of YoE.

**Verification:** call `preview_ats_extraction` in each scenario; check heading order in the returned text.

---

## 7. Missing-sections migration (MCP)

**Scenario:** A pre-feature resume (raw only, no sidecar) is used in a session.

**Steps:**
1. Manually delete `inputs/<label>.sections.json` if it exists.
2. `load_jd` + `submit_keywords`.

**Expected (MCP mode):**
- `submit_keywords` returns an error envelope:
  ```json
  {"error_code":"sections_missing","data":{"raw":"<raw text>","hint":"call add_resume with sections"}}
  ```
- Orchestrator reads `raw`, parses sections, re-calls `add_resume` with `sections`.
- Next `submit_keywords` succeeds.

**Expected (Headless mode):**
- `submit_keywords` internally calls `Orchestrator.ParseSections`, writes the sidecar atomically, and succeeds transparently. No orchestrator loop required.

---

## 8. preview_ats_extraction identity round-trip

**Steps:**
```text
preview_ats_extraction session_id=<id>
```

**Expected today:** returned `extracted_text` equals `Renderer.Render(sections)` byte-for-byte (identity Extractor). Tomorrow, when a real PDF extractor is plugged in, the same tool signature continues to work with format-specific output. Callers need not change.

---

## 8a. SC-004 renderer seam verification (architectural gate)

**Scenario:** A maintainer adds a new `Renderer` implementation without touching scoring, tailoring, or MCP code.

**Steps:**
1. In a `_test.go` file under `internal/service/render/`, define a minimal `altRenderer` struct that satisfies `port.Renderer` (`Render(*model.SectionMap) (string, error)` returning a fixed string).
2. Wire `altRenderer` into the service graph in place of the default renderer (local test only).
3. Run `go build ./...`.

**Expected:**
- Build succeeds with zero changes to `internal/service/scorer/`, `internal/service/tailor/`, or `internal/mcpserver/`.
- The `var _ port.Renderer = (*Service)(nil)` compile-time guard in `internal/service/render/` confirms the interface contract is stable.

**Fail signal:** Any change required outside `internal/service/render/` or `internal/port/render.go` to make the build pass.

---

## 9. Migration / backward compatibility

- Existing resumes lacking `.sections.json` continue to be listed by `ListResumes()`.
- The first operation that requires sections triggers the per-mode recovery path (§7).
- `schema_version: 1` written on every SaveSections; read-time mismatch returns `sections_unsupported_schema` with the persisted version exposed in the envelope `data`.

---

## 10. Constitution gates

Run before merging:
```bash
go test ./...             # TDD gate — must pass, ≥80% coverage across touched packages
go vet ./...              # Hexagonal violation detector (presenter → service forbidden import checks)
go test -tags integration ./...   # Integration gate — renderer/extractor end-to-end + MCP wire format
```

**All test tiers must pass.** `go test -race ./...` must pass for any test that exercises the session store.

---

## 11. Manual smoke

The original PlayStation failure:
1. `add_resume` with sections containing `"Skills & Abilities"` content under `sections.skills` (kind: categorized OK).
2. `load_jd jd_url="<PS data platform JD>"`.
   > **JD fixture**: Store the PlayStation Data Platform JD text in `testdata/jd_playstation_dataplatform.txt` at repo root so this step can be replayed offline via `load_jd jd_raw_text=<contents of that file>` if the URL is no longer live.
3. `submit_keywords` — verify `sections` is present in the response and `req_matched` includes `Apache Spark` via alias expansion from resume's `PySpark`.
4. `submit_tailor_t1` — edits apply, not rejected.
5. Final score ≥ 70 or graduation to T2.
6. `preview_ats_extraction` — identity text matches what the scorer consumed.

If all nine steps above plus this smoke run succeed, the feature is complete per FR-001 through FR-D05.
