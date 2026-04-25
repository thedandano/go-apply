# Contract: Section Registry (`render.Service.Render`)

## Interface

`render.Service.Render(sections *model.SectionMap) (string, error)`

The implementation iterates an ordered `[]sectionWriter` slice. Each writer is called in slice order. Writers for empty sections must be no-ops (no output, no error).

## Registry slice contract

```go
var sectionWriters = []sectionWriter{
    {"contact",        func(b *strings.Builder, s *model.SectionMap) { writeContact(b, &s.Contact) }},
    {"summary",        func(b *strings.Builder, s *model.SectionMap) { writeSection(b, "SUMMARY", func() { b.WriteString(s.Summary + "\n") }, s.Summary != "") }},
    {"experience",     func(b *strings.Builder, s *model.SectionMap) { writeExperience(b, s.Experience) }},
    {"education",      func(b *strings.Builder, s *model.SectionMap) { writeEducation(b, s.Education) }},
    {"skills",         func(b *strings.Builder, s *model.SectionMap) { writeSkills(b, s.Skills) }},
    {"projects",       func(b *strings.Builder, s *model.SectionMap) { writeProjects(b, s.Projects) }},
    {"certifications", func(b *strings.Builder, s *model.SectionMap) { writeCertifications(b, s.Certifications) }},
    {"awards",         func(b *strings.Builder, s *model.SectionMap) { writeAwards(b, s.Awards) }},
    {"volunteer",      func(b *strings.Builder, s *model.SectionMap) { writeVolunteer(b, s.Volunteer) }},
    {"publications",   func(b *strings.Builder, s *model.SectionMap) { writePublications(b, s.Publications) }},
    // Tier 4
    {"languages",      func(b *strings.Builder, s *model.SectionMap) { writeLanguages(b, s.Languages) }},
    {"speaking",       func(b *strings.Builder, s *model.SectionMap) { writeSpeaking(b, s.Speaking) }},
    {"open_source",    func(b *strings.Builder, s *model.SectionMap) { writeOpenSource(b, s.OpenSource) }},
    {"patents",        func(b *strings.Builder, s *model.SectionMap) { writePatents(b, s.Patents) }},
    {"interests",      func(b *strings.Builder, s *model.SectionMap) { writeInterests(b, s.Interests) }},
    {"references",     func(b *strings.Builder, s *model.SectionMap) { writeReferences(b, s.References) }},
}
```

## Invariants

1. **No empty headers**: every `writeX` function for a slice-typed section must guard with `len(slice) == 0` and return immediately if empty.
2. **Render output is identical for existing sections**: iterating the slice must produce byte-for-byte identical output compared to the pre-refactor 10-arm hardcoded dispatch. Verified by a golden test or by comparing before/after on a known fixture.
3. **Order is authoritative**: the slice index determines section order. Future specs that reorder sections do so by changing slice positions, not by adding a sort key.
4. **The `key` field is informational only in this spec**: no logic branches on `key` values here. It is reserved for Spec C's template system.
5. **Adding a new section = one appended entry**: no other code in `render.go` requires modification. The `Render` method loop is generic.

## Writer function signature

```go
func writeX(b *strings.Builder, items []XEntry) {
    if len(items) == 0 {
        return
    }
    // emit "SECTION HEADING\n"
    // emit each entry
}
```

The `strings.Builder` is passed by pointer. Writers must not reset or replace it.

## Tier 4 heading strings (authoritative)

| Section key | Rendered heading |
|---|---|
| `languages` | `LANGUAGES` |
| `speaking` | `SPEAKING ENGAGEMENTS` |
| `open_source` | `OPEN SOURCE` |
| `patents` | `PATENTS` |
| `interests` | `INTERESTS` |
| `references` | `REFERENCES` |

All-caps, matching existing section convention (`SUMMARY`, `EXPERIENCE`, `SKILLS`, etc.). Golden tests must assert these exact strings.
