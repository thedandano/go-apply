# Data Model: T1 Category-Aware Skills Edits

**Branch**: `005-fix-t1-categorized-skills` | **Date**: 2026-04-25

## Changed Entities

### `port.Edit` — add optional `Category` field

**File**: `internal/port/tailor.go`

**Current shape**:
```go
type Edit struct {
    Section string `json:"section"`
    Op      EditOp `json:"op"`
    Target  string `json:"target,omitempty"`
    Value   string `json:"value,omitempty"`
}
```

**New shape**:
```go
type Edit struct {
    Section  string `json:"section"`
    Op       EditOp `json:"op"`
    Target   string `json:"target,omitempty"`
    Value    string `json:"value,omitempty"`
    Category string `json:"category,omitempty"` // required when section="skills" and skills.kind="categorized"
}
```

**Invariants**:
- `Category` is optional (`omitempty`): omitting it on a flat resume is valid and unchanged behavior
- `Category` is required when the target resume's skills section has `kind = "categorized"` — enforced at runtime in `applySkillsEdit`, not at the struct level

---

### `applySkillsEdit` — routing logic

**File**: `internal/service/tailor/apply_edits.go`

**Behavior change**:

```
Before:
  if kind == categorized → reject all ops

After:
  if kind == categorized:
    if Category == "" → reject, list available categories
    if Category not in Categorized map → reject, list available categories
    switch op:
      add     → parse value by comma+trim, append items to Categorized[Category]
      replace → parse value by comma+trim, set Categorized[Category] = items
      default → reject (unsupported op for skills)
  else (kind == flat):
    existing flat logic unchanged
```

**Value parsing contract** (for both `add` and `replace` on categorized):
- Split `value` on `,`
- Trim leading/trailing whitespace from each token
- Discard empty tokens (e.g., trailing comma)
- Result is `[]string` of individual skill items

**Rejection message format** (for missing/unknown category):
- Missing: `"op %q on categorized skills requires a category; available: [%s]"`
- Unknown:  `"category %q not found; available: [%s]"`
- Where `[%s]` is the sorted comma-separated list of category names from the map

---

## Unchanged Entities

- `SkillsSection` struct: no changes (`map[string][]string` already the correct shape)
- `EditOp` constants: no additions
- `EditResult`, `EditRejection`: no changes
- `port.Tailor` interface: signature unchanged (`ApplyEdits` accepts `[]Edit`)
- On-disk sidecar format: unchanged

---

## State Transitions

```
Categorized skills section after op=add:
  Before: {"Backend & Data": ["Go", "PostgreSQL"]}
  Edit:   {section:"skills", category:"Backend & Data", op:"add", value:"Apache Kafka, Spark"}
  After:  {"Backend & Data": ["Go", "PostgreSQL", "Apache Kafka", "Spark"]}

Categorized skills section after op=replace:
  Before: {"Backend & Data": ["Go", "PostgreSQL"]}
  Edit:   {section:"skills", category:"Backend & Data", op:"replace", value:"Go, PostgreSQL, Apache Kafka"}
  After:  {"Backend & Data": ["Go", "PostgreSQL", "Apache Kafka"]}

Flat skills after any edit:
  Unchanged — Category field silently ignored
```
