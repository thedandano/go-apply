# Data Model: Pretty Log Output

**Date**: 2026-04-25

No persistent entities. This feature is a pure display-layer transform with no storage impact.

## Transform Contract

**Input**: Raw log line (UTF-8 string) from a charmbracelet/log logfmt file  
Format: `YYYY-MM-DD HH:MM:SS LEVEL message [key=value ...]`  
JSON fields: `key="<Go-quoted JSON object or array>"`

**Output**: One or more display lines written to `io.Writer`  
- Header line: input line with JSON-valued fields removed; trailing whitespace trimmed  
- Per JSON field (in original order):
  ```
    key:
      {
        "field": "value"
      }
  ```

## Field Classification

| Field type | Example | Renderer action |
|------------|---------|-----------------|
| Unquoted value | `tool=submit_tailor_t2` | Kept on header line |
| Quoted non-JSON string | `status="ok"` | Kept on header line |
| Quoted JSON object | `result="{\"score\":75}"` | Removed from header; printed below under `result:` label |
| Quoted JSON array | `items="[1,2,3]"` | Removed from header; printed below under `items:` label |
| Quoted malformed JSON | `data="{broken"` | Kept on header line (passes through unchanged) |
