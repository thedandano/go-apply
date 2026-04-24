# Contract: submit_tailor_t1

## Before (current)

**Input parameter**: `skill_adds` (string array)
```json
{
  "session_id": "abc123",
  "skill_adds": "[\"Apache Kafka\", \"Databricks\", \"CI/CD pipelines\"]"
}
```

**Response data**:
```json
{
  "added_keywords": ["Apache Kafka", "Databricks"],
  "skills_section_found": true,
  "previous_score": 50.2,
  "new_score": { ... },
  "tailored_text": "..."
}
```

## After (this feature)

**Input parameter**: `skill_rewrites` (array of `{original, replacement}` objects)
```json
{
  "session_id": "abc123",
  "skill_rewrites": "[{\"original\": \"Docker, Kubernetes\", \"replacement\": \"Docker, Kubernetes, EKS\"}, {\"original\": \"CI/CD\", \"replacement\": \"Apache Kafka, CI/CD\"}]"
}
```

**Response data**:
```json
{
  "substitutions_made": 2,
  "skills_section_found": true,
  "previous_score": 50.2,
  "new_score": { ... },
  "tailored_text": "..."
}
```

## Validation rules

| Rule | Error code | Condition |
|------|-----------|-----------|
| Required | `missing_skill_rewrites` | parameter absent or empty string |
| Parse | `invalid_skill_rewrites` | JSON parse failure |
| Non-empty | `empty_skill_rewrites` | parsed array length == 0, OR all entries have `original == ""` |
| Cap | `too_many_rewrites` | `len(rewrites) > MaxTier1SkillRewrites` |

## submit_keywords contract addition

**New response field** (`skills_section`):
```json
{
  "extracted_keywords": { ... },
  "scores": { ... },
  "best_resume": "senior_fullstack_backend",
  "best_score": 50.2,
  "skills_section": "SKILLS & ABILITIES\nLanguages: Python, Java, SQL\nBackend & Data: ..."
}
```
Field is omitted (`omitempty`) when no Skills section is found in the best resume.
