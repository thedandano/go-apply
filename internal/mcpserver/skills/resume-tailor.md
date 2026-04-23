---
name: resume-tailor
version: 1.1
description: >
  Modifies a resume to improve keyword match against a job description via a two-tier system.
  Tier 1 adds honest keywords to the Skills section from skills_reference.md. Tier 2 rewrites
  up to 4 experience bullets using an accomplishments doc (SBI format) to surface missing
  keywords. Outputs a modified .pdf file via a template-based Python script.

  Auto-triggered by job-fit-analyzer when score is 40-69. Also use when the user says
  "tailor my resume", "improve my resume for this job", "optimize my resume", "make my
  resume stronger for this role", "customize my resume", "modify resume", "update resume
  for this job", or wants to add skills from skills_reference or rewrite bullets from
  an accomplishments doc.
---

# Resume Tailor

**First-time setup:** run `bash resume-tailor/setup.sh` from the `skills/` directory to install Python dependencies and verify a LaTeX compiler is available.

You are running a deterministic resume modification pipeline. Follow this framework exactly.

---

## Golden Rule

**NEVER** add keywords, bullets, metrics, or claims to a resume that have no basis in the
candidate's actual experience. This rule overrides all other instructions. If a JD keyword
cannot be traced to one of these sources, it must NOT be added:

1. Accomplishments doc (SBI format)
2. skills_reference.md
3. Existing resume bullets

If in doubt, skip the keyword and log it as "no basis found."

---

## Voice and Honesty Standards

These apply to every bullet written or rewritten, regardless of tier.

**Write like a human, not a status report.**
Use active verbs that show ownership: built, launched, cut, shipped, led, designed.
Avoid corporate passive constructions: "responsible for", "leveraged", "utilized", "supported the launch of".
A bullet should feel like the candidate telling you what they did, not a job description listing duties.

**Claim only what the candidate owned.**
If a partner team shipped it, say so: "prototyped X, which a partner team took to production."
If it was a hackathon or prototype, qualify it: "prototyped", "explored", "validated in a hackathon setting." A 2-day hackathon win is genuinely impressive — but it's hackathon scale, not production scale. Don't let "we won an internal hackathon" become "I built and deployed a production AI system." The former is true and still strong; the latter is fabrication.
If results are not yet confirmed live (e.g., SEO not yet indexed, feature not yet released), qualify: "established the foundation for", "designed to enable."
Scope check: could the candidate explain this bullet in a behavioral interview without saying "well, the team I supported did..."? If not, reframe or skip.

**Quantify carefully.**
Distinguish between production metrics, pilot/hackathon scope, and estimated impact.
Never inflate a prototype metric to production scale.
If no metric exists in the accomplishments doc, describe impact qualitatively rather than inventing numbers.

**Avoid em-dashes and prose double-hyphens.**
Em-dashes and " -- " used as prose separators read as AI-generated to modern detectors. Use commas, colons, semicolons, or periods instead. The " -- " separator is acceptable ONLY in date ranges (e.g., "April 2022 -- November 2024") and section/role title lines.

The `resume_modifier.py` script automatically sanitizes these after every modification pass via `check_and_sanitize_dashes_latex()`. It replaces prose ` -- ` with `, ` in bullet and summary content, logs what it changed, and leaves date ranges untouched. Do not rely solely on the sanitizer — write clean content to begin with.

**Preserve the candidate's template.**
When generating output, use the candidate's existing template file -- don't revert to a generic layout.
Respect formatting choices already in the template (spacing, font, section order).

---

## Operating Rules

1. Source of truth priority: accomplishments doc > skills_reference.md > existing resume bullets.
2. Rescore after EVERY modification tier using `/mnt/skills/user/job-fit-analyzer/scripts/score.py` via `--resume-files` with the modified `.pdf` path.
3. Output a changelog with every modification -- one line per change, noting what was
   added/rewritten and which JD keyword it targets.
4. Preserve resume formatting, structure, and voice. Do not rewrite content that is already
   keyword-matched. Only modify what is necessary to close gaps.
5. Never use em dashes in any generated text. Use " -- " (space-hyphen-hyphen-space) instead.
6. When called by the analyzer (auto-trigger), all inputs are inherited from the analyzer's
   current run context: JD keywords, resume text, scoring output, skills_reference path,
   accomplishments doc path, candidate years, required years, seniority match.
7. When called manually by the user, gather inputs explicitly (see Manual Trigger Mode below).
8. The script is the source of truth for page count. If `resume_modifier.py` returns
   `"success": false` due to page overflow, read `actual_pages`, decide what to cut using
   the cut priority rules in the File Generation section, update the spec, and rerun ONCE.
   If it fails again, surface the `"error"` field to the user and stop.

---

## Template Management

The script accepts `.tex` (preferred) or `.docx` templates via `template_path` in the spec.

### Canonical Template Generation (Recommended for new candidates)

Use `template_generator.py` to create a canonical `.tex` template from any existing resume
format (PDF, DOCX, or TEX). This is the preferred path for onboarding a new candidate or
regenerating a fresh template from scratch.

```bash
uv run python /mnt/skills/user/resume-tailor/scripts/template_generator.py \
  --input  "/path/to/existing_resume.pdf" \
  --output "/path/to/candidate_resume.tex" \
  --name   "Full Name" \
  --location "City, State" \
  --email  "email@example.com" \
  --linkedin "linkedin.com/in/handle"
```

**Optional contact fields** (omit any you don't want to appear):
- `--phone "555-555-5555"` -- adds phone number to contact line
- `--website "https://yoursite.com"` -- adds personal website as a hyperlink
- `--years 4` -- override inferred years of experience (used for page-limit enforcement)

**Page rules enforced automatically:**
- `--years < 5`: 1-page maximum enforced. Script errors if compiled output exceeds 1 page.
- `--years >= 5`: up to 2 pages allowed.

**The script uses a skeleton-based `{{placeholder}}` approach.** It reads
`resume_skeleton.tex` (bundled alongside the script) and replaces each token with
rendered LaTeX content extracted from the source resume. The skeleton encodes all
validated formatting: margins, fonts, section header commands, spacing, and hyperlink
colors. This means the output template will always match the canonical style regardless
of the source format.

**Skeleton tokens:**
- `{{name}}` -- candidate's full name (large bold, centered)
- `{{contact_line}}` -- pipe-separated contact info with optional hyperlinks
- `{{section_headline}}` -- professional summary / headline paragraph
- `{{section_skills}}` -- skills section with `\skillcat{}` category labels
- `{{section_experience}}` -- work history with role blocks, bullets, and dates
- `{{section_projects}}` -- project entries (same structure as experience)
- `{{section_volunteer}}` -- volunteer work entries
- `{{section_education}}` -- education entries (centered, single line)

**`--dry-run`**: prints the filled `.tex` to stdout without writing or compiling. Useful
for inspecting extracted content before committing.

**`--no-pdf`**: writes the `.tex` file but skips xelatex compilation. Use when you only
need the template source and will compile later via `resume_modifier.py`.

### Existing conversion path (for candidates already on `.docx`)

**One-time conversion** (run once to create a `.tex` from an existing Word file):
```bash
uv run python /mnt/skills/user/resume-tailor/scripts/resume_modifier.py \
  --convert-template "/path/to/Candidate_Resume.docx"
```
Output: `Candidate_Resume.tex` alongside the `.docx`.

**Normal path** -- point `template_path` at the `.tex` file for fast compilation without re-conversion.

**Ad-hoc path** -- point `template_path` at `.docx`. The script auto-converts on first run and caches the `.tex` sidecar. Subsequent runs reuse the cache unless the `.docx` is newer or `"force_convert": true` is set.

**LaTeX compiler required**: install `tectonic` (preferred) or `texlive-xetex`. If absent, the script returns `{"success": false, "error": "No LaTeX compiler found..."}`.

**Spacing and formatting auto-fixes**: `resume_modifier.py` automatically injects `\frenchspacing` into any template that's missing it. This prevents LaTeX's default behavior of adding extra space after sentence-ending periods (which causes visible mid-sentence gaps like "80%.  I want..."). It also runs the dash sanitizer on every pass. These are silent fixes — they'll appear in `skipped_reasons` in the script output if triggered.

---

## Tier 1 -- Skills Section Update (Light Touch)

**Trigger:** Auto (score 40-69) or manual request.
**Scope:** Skills section ONLY. No bullet rewrites. Summary is handled in Summary Assessment after all tiers complete.
**Approval:** Automatic -- no user approval needed.

### Process:

1. From the analyzer's scoring output, identify `keyword_detail.req_unmatched` and
   `pref_unmatched` (the JD keywords not found in the resume).
2. For each unmatched keyword, check if it exists in `skills_reference.md`.
   - Use exact string matching first.
   - If no exact match, check for standard abbreviations (e.g., "CI/CD" matches
     "Continuous Integration and Continuous Delivery (CI/CD)").
3. If found in skills_reference: add it to the appropriate Skills subsection in the resume.
   Determine the correct subsection by matching the keyword's category in skills_reference.md
   (e.g., "Kafka" goes under "Backend & Data" or similar).
4. If NOT found in skills_reference: do NOT add it. Log it as "not in candidate's skill set."
5. Do not remove any existing skills from the resume.
6. Do not reorder existing skills. Add new ones at the end of the relevant subsection.

### Output:

```
TIER 1 CHANGELOG:
- Added [keyword] to [Skills subsection] -- targets JD [required/preferred] keyword
- Added [keyword] to [Skills subsection] -- targets JD [required/preferred] keyword
- Skipped [keyword] -- not in skills_reference.md
- Skipped [keyword] -- not in skills_reference.md
```

### Rescore:

Run `/mnt/skills/user/job-fit-analyzer/scripts/score.py` via `--resume-files [modified_pdf_path]` and `--resume-labels [label]`. Use the same `--required`, `--preferred`, `--candidate-years`, `--required-years`, `--seniority-match`, and `--reference-file` args as the original scoring call.
Use the same arguments as the original scoring call, but with updated `--resume-text`.

**CRITICAL:** Resume text passed to the script must include explicit line breaks and
bullet-style formatting using `-` prefixes. Passing unformatted text blocks causes
impact metric detection to return 0.0.

Report: new score, delta from original, and which keywords now match.

DO NOT generate a resume file after Tier 1. File generation happens once after the
last tier completes. Tier 1 only records what skills to add and rescores.

---

## Tier 2 -- Bullet Rewriting (Heavy Touch)

**Trigger:** Score still < 70 after Tier 1.
**Scope:** Experience bullets + Skills section updates from new bullet content. Summary is handled in Summary Assessment after all tiers complete.
**Approval:** Automatic -- no user approval needed.
**Prerequisite:** Accomplishments doc must exist (path from analyzer Step 0D).
If no accomplishments doc: do NOT run Tier 2. Exit with advisory:
> "Tier 2 requires an accomplishments doc but none was found. Provide one to enable
> deeper tailoring."

### Process:

1. Identify remaining `req_unmatched` keywords after Tier 1 rescore.
2. Load the accomplishments doc using the path from the analyzer's Step 0D.
   - For `.docx` files: extract text using
     `python3 -c "import docx; doc=docx.Document('[path]'); print('\n'.join([p.text for p in doc.paragraphs]))"`
   - For `.md` files: read directly.
3. For each unmatched keyword (up to the rewrite cap):
   a. Search the accomplishments doc for bullets/paragraphs that demonstrate this skill.
      Look for the keyword itself, synonyms, and contextual descriptions of the skill.
   b. If found in accomplishments doc:
      **SCOPE CHECK (mandatory):** Verify the keyword describes work the candidate
      actually performed -- not adjacent or downstream work by another team.
      Test: Could the candidate explain this keyword in a behavioral interview
      without saying "well, my pipeline was used for..." or "the team I supported..."?
      If scope check fails: skip and log as "scope check failed -- candidate did not
      own this work directly."
      - Identify the weakest resume bullet in the relevant role section.
        "Weakest" = lowest relevance to the JD, using:
        `relevance_score(bullet) = 0.6 * keyword_overlap(bullet, jd_keywords) + 0.4 * metric_flag(bullet)`
        where keyword_overlap = fraction of JD keywords present in the bullet,
        and metric_flag = 1 if bullet contains a quantitative metric, 0 otherwise.
      - Rewrite that bullet to incorporate the missing keyword naturally, using the
        accomplishments doc content as the source of truth for what actually happened.
      - If the accomplishments doc has a stronger metric than the current bullet, use it.
   c. If NOT found in accomplishments doc:
      - Check if the keyword can be woven into an existing bullet without fabricating
        experience. Example: if the candidate used "event-driven architecture" (in
        skills_reference) and a bullet describes Step Functions workflows, adding
        "event-driven" is honest.
      - If no honest rewrite is possible: skip and log.
4. After bullet rewrites, update the Skills section to include any new keywords that
   now appear in bullets but are not yet in Skills.
5. **Voice preservation:** Match the existing resume's tone and structure:
   - If bullets start with past-tense action verbs, maintain that.
   - If bullets follow a pattern of [action] + [object] + [scale/metric], maintain that.
   - Do not introduce new structural patterns.

### Rewrite Cap:

**Maximum 4 bullet rewrites per Tier 2 run.** If more than 4 keywords remain unmatched,
prioritize by: required keywords before preferred, then by frequency in JD (keywords
mentioned multiple times get priority).

### Constraints:

- Never fabricate metrics. If the accomplishments doc has a metric, use it. If not,
  describe the impact qualitatively.
- Never add a keyword that the candidate cannot defend in an interview.
- Never rewrite a bullet that already matches a JD keyword (it's already doing its job).

### Output:

```
TIER 2 CHANGELOG:
- Rewrote bullet [N] in [Role]: "[old first 10 words...]" -> "[new first 10 words...]"
  Targets: [keyword]. Source: accomplishments doc [section name]
- Rewrote bullet [N] in [Role]: "[old first 10 words...]" -> "[new first 10 words...]"
  Targets: [keyword]. Source: existing bullet reframe
- Added [keyword] to Skills section (now appears in rewritten bullet [N])
- Skipped [keyword] -- scope check failed (candidate did not own this work directly)
- Skipped [keyword] -- no basis in accomplishments or experience
```

### Rescore:

Same as Tier 1. Run score.py, report delta.

---

## Summary Assessment

Runs after ALL tiers complete, before file generation. This is the only place summary
changes happen -- never in Tier 1 or Tier 2.

**Trigger:** Either of these conditions:
- A `summary_alignment_note` was passed in from the analyzer (e.g., "JD emphasizes speed
  and scrappiness; current summary leads with correctness and rigor")
- The user explicitly asked to update the summary in manual trigger mode

**If neither condition is true:** skip this step. Leave `summary_replacement: null`.

**Process:**

1. Read the current summary from the resume.
2. Read the final state of all modified bullets (from Tier 1/2 changelogs).
3. Using the `summary_alignment_note` and the final bullet content as context, draft a
   replacement summary that:
   - Shifts emphasis to align with the JD's values/work style
   - Remains coherent with the final bullet content (not just the original)
   - Preserves the candidate's voice and makes no fabricated claims
   - Stays concise (3-4 sentences max)
4. Record the target section header (e.g., "SUMMARY", "PROFILE") as `summary_section_header`
   so the script can locate it precisely. If the resume has no explicit summary header,
   set to null and the script will fall back to the first long non-bullet paragraph.

**Changelog:**
```
SUMMARY ASSESSMENT:
- Updated: shifted emphasis from [old trait] to [new trait]
  Reason: [summary_alignment_note or user request]
- OR: Skipped -- no alignment gap detected
```

No rescore needed. Summary does not affect keyword scoring.

---

## File Generation

The pipeline from all completed steps flows into a **single** `resume_modifier.py` call.
Never call the script per-tier or before Summary Assessment completes.

### Step 1 -- Build the combined spec

Combine ALL changes from all tiers that ran:

```json
{
  "template_path": "/mnt/user-data/uploads/Candidate_Resume_backend.tex",
  "output_path": "/mnt/user-data/outputs/Resume_[FirstName]_[LastName]_[Company].pdf",
  "max_pages": 1,
  "skills_modifications": {
    "add": [
      {"keyword": "REST APIs", "subsection": "Backend & Data"}
    ],
    "remove": []
  },
  "bullet_modifications": [
    {
      "role": "Software Development Engineer II",
      "original_prefix": "Owned end-to-end distributed",
      "replacement": "Full replacement bullet text here."
    }
  ],
  "sections_to_cut": [],
  "summary_replacement": null,
  "summary_section_header": null
}
```

For the full list of spec fields and their types, run:
```bash
python3 /mnt/skills/user/resume-tailor/scripts/resume_modifier.py --print-schema
```

**Template files:**
Use the candidate's existing resume file as the template. Check the project inputs folder for available variants (e.g., backend, data, fullstack). If multiple exist, ask the user which to use before proceeding. Prefer `.tex` files over `.docx` for faster compilation and more reliable formatting.

### Step 2 -- Call the script

Write the spec to a temp file, then call:

```bash
python3 /mnt/skills/user/resume-tailor/scripts/resume_modifier.py \
  --template "/path/to/source_resume.docx" \
  --spec     "/tmp/modification_spec.json" \
  --output   "/mnt/user-data/outputs/Resume_[FirstName]_[LastName]_[Company].pdf"
```

`--template` and `--output` override the matching fields in the spec if both are provided.
The script returns JSON to stdout. Parse it immediately.

### Step 3 -- Handle the result

**On `"success": true`:** Present the `.pdf` via `present_files`.

**On `"success": false` (page overflow):** The `"error"` field will read:
`"Page count exceeded: N page(s). max_pages=1. Resume was NOT written."`

Apply cut priority (in order, stop when page count is resolved):
1. `sections_to_cut: ["Projects"]` -- only if all keyword-bearing content is already in
   experience bullets. Keyword impact: none/low.
2. `trim_old_bullets: true` -- trims older roles to 3 bullets each. Keyword impact: low.
3. `shorten_summary: true` -- removes last sentence of summary paragraph. Use only if
   `summary_replacement` was NOT set (don't undo a deliberate summary rewrite).

Flag any cut that removes a keyword, and rescore after the rerun.

Update the spec with the cut fields and rerun the script once. If it still fails, surface
the error to the user and stop.

**On any other `"success": false`:** Surface `"error"` verbatim. Do not attempt a fix inline.

**Script-missing fallback:** If `resume_modifier.py` cannot be found or import-fails,
output the spec as a JSON code block with:
> "The resume_modifier.py script could not run. Here is the modification spec -- apply
> these changes manually or re-upload the script to the project."
Do NOT write generation code inline.

---

## Manual Trigger Mode

When called directly by the user (not auto-triggered by the analyzer):

1. Ask one clarifying question before starting:
   > "Is this resume for this specific role only, or should changes be broadly
   > applicable to similar backend/data roles?"
2. If role-specific: optimize purely for this JD's keyword match.
3. If broadly applicable: only add keywords that are cross-role relevant.
   Flag role-specific changes separately in the changelog with a [role-specific] tag.
4. Gather required inputs:
   - JD: ask user to provide URL, paste text, or reference a cached JD.
   - Resume: use resumes found in project (or ask user to specify).
   - skills_reference.md: check project root.
   - Accomplishments doc: check project root.
5. Run the analyzer's scoring first to establish baseline, then proceed with the
   Tier 1 -> Tier 2 cascade as normal.

When auto-triggered by the analyzer: default to role-specific. No question asked.
All inputs inherited from the analyzer's current context.
