# MCP Tool Contract: `submit_tailor_t1`

**Changed fields**: `edits` description string, workflow prompt

## Tool Registration (server.go)

**Current description**:
```
JSON array of {"section":"skills","op":"replace|add","value":"..."} objects.
Max 5 entries. e.g. [{"section":"skills","op":"add","value":"GCP"}]
```

**New description**:
```
JSON array of {"section":"skills","op":"replace|add","value":"...","category":"..."} objects.
Max 5 entries. "category" is required when skills.kind="categorized" — use a key from
sections.skills.categorized returned by submit_keywords.
e.g. flat:        [{"section":"skills","op":"add","value":"GCP"}]
     categorized: [{"section":"skills","category":"Cloud","op":"add","value":"GCP, Azure"}]
```

## Workflow Prompt (prompt.go)

### Tool table row change

**Current**:
```
| submit_tailor_t1 | Apply structured edits to the Skills section and rescore |
  session_id (req), edits (JSON array of {section:"skills", op, value}, req)
```

**New**:
```
| submit_tailor_t1 | Apply structured edits to the Skills section and rescore |
  session_id (req), edits (JSON array of {section:"skills", op, value, category?}, req)
```

### Step 5 T1 instruction change

**Current**:
```
2. Call submit_tailor_t1 with edits: [{"section":"skills","op":"replace","value":"AWS, GCP"}, ...]
   (max 5 items). Use prefer one-for-one replace over pure add to keep section length stable
   (e.g. replace "AWS" with "AWS, GCP"). Section must be "skills".
```

**New** (add conditional paragraph after existing instruction):
```
2. Call submit_tailor_t1 with edits (max 5 items). Section must be "skills".
   - Flat skills (skills_section.kind == "flat"): omit category.
     e.g. {"section":"skills","op":"replace","value":"AWS, GCP"}
   - Categorized skills (skills_section.kind == "categorized"): include category matching
     a key from sections.skills.categorized (from submit_keywords response).
     e.g. {"section":"skills","category":"Cloud","op":"add","value":"AWS, GCP"}
   Prefer one-for-one replace over pure add to keep section length stable.
```

## Behavioral Contract

| Input state | `category` field | Expected outcome |
|---|---|---|
| `kind=flat` | omitted | Edit applied to flat string |
| `kind=flat` | present | `category` ignored; edit applied to flat string |
| `kind=categorized` | valid category name | Edit applied to that category's `[]string` list |
| `kind=categorized` | omitted | Rejected — message lists available categories |
| `kind=categorized` | unknown name | Rejected — message lists available categories |
