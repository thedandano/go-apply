# Internal Port Contracts — ATS-Aware Resume Tailoring

Go interface signatures for every new or extended port. All live under `internal/port/` per Constitution III (Hexagonal Architecture). Adapters must satisfy these interfaces via `var _ port.Foo = (*Service)(nil)` compile-time checks.

---

## 1. `port.Renderer` (new — `internal/port/render.go`)

```go
package port

import "github.com/thedandano/go-apply/internal/model"

// Renderer converts a structured SectionMap into output text or artifacts.
// The default plain-text implementation is used by scoring and final output today;
// future PDF/DOCX implementations plug in without altering callers.
type Renderer interface {
    // Render returns the rendered output for the given sections.
    // Returns a non-nil error if the SectionMap is invalid (e.g., missing required
    // Contact.Name). The Renderer does not mutate its input.
    Render(sections *model.SectionMap) (string, error)
}
```

**Default adapter:** `internal/service/render/Service`.
- Order selection: `sections.Order` if non-empty; otherwise tier selected by YoE (research §R1).
- Headings: canonical labels only (research §R7).
- Output format: markdown with `## <Heading>` and `- ` bullets.

---

## 2. `port.Extractor` (new — `internal/port/extract.go`)

```go
package port

// Extractor converts a rendered artifact into the text an ATS would extract.
// The default identity implementation returns its input verbatim. Future real
// implementations (pdftotext, Tika) plug in without altering callers.
type Extractor interface {
    // Extract returns the ATS-equivalent extracted text of the rendered input.
    // Returns a non-nil error if extraction fails (e.g., malformed PDF).
    Extract(rendered string) (string, error)
}
```

**Default adapter:** `internal/service/extract/IdentityService`.
- `Extract(s) == (s, nil)` always.

---

## 3. `port.Orchestrator` (extended — `internal/port/orchestrator.go`)

Add one method:

```go
type Orchestrator interface {
    ExtractKeywords(ctx context.Context, input ExtractKeywordsInput) (model.JDData, error)
    PlanT1(ctx context.Context, input *PlanT1Input) (PlanT1Output, error)
    PlanT2(ctx context.Context, input *PlanT2Input) (PlanT2Output, error)
    GenerateCoverLetter(ctx context.Context, input *CoverLetterInput) (string, error)

    // ParseSections converts raw resume text into a structured SectionMap via the
    // configured LLM. The Headless (CLI) adapter implements this; the MCP adapter
    // returns model.ErrNotSupportedInMCPMode because in MCP mode the orchestrator
    // (Claude) supplies sections directly at onboarding.
    ParseSections(ctx context.Context, rawResume string) (model.SectionMap, error)
}
```

**Headless adapter** (`internal/service/orchestrator/`):
- Sends raw text to `LLMClient.ChatComplete` with a JSON-schema-constrained system prompt describing the SectionMap.
- Parses response, runs `model.ValidateSectionMap`, returns.
- On validation failure: wraps with `fmt.Errorf("parse sections: %w", err)` and returns the SchemaError — does not retry.

**MCP adapter:** returns `model.ErrNotSupportedInMCPMode` (stub); not called in normal MCP operation because MCP onboarding writes sections directly.

---

## 4. `port.ResumeRepository` (extended — `internal/port/resume.go`)

```go
type ResumeRepository interface {
    ListResumes() ([]model.ResumeFile, error)

    // LoadSections returns the structured SectionMap for the given label.
    // Returns model.ErrSectionsMissing if the sections sidecar is absent.
    LoadSections(label string) (model.SectionMap, error)

    // SaveSections persists the SectionMap for the given label, atomically
    // replacing any prior sidecar.
    SaveSections(label string, sections model.SectionMap) error
}
```

**FS adapter** (`internal/repository/fs/resume.go`):
- `LoadSections(label)` reads `<dataDir>/inputs/<label>.sections.json`; decodes to `SectionMap`; runs `ValidateSectionMap`; returns `ErrSectionsMissing` on ENOENT.
- `SaveSections(label, s)` writes to temp file + rename for atomicity; file permission `config.FilePerm`.

---

## 5. `port.Tailor` (extended — `internal/port/tailor.go`)

Two new types and one new method (existing `TailorResume` stays for backward compat during migration; removed at feature end):

```go
type EditOp string

const (
    EditOpAdd     EditOp = "add"
    EditOpRemove  EditOp = "remove"
    EditOpReplace EditOp = "replace"
)

type Edit struct {
    Section string `json:"section"`
    Op      EditOp `json:"op"`
    Target  string `json:"target,omitempty"`
    Value   string `json:"value,omitempty"`
}

type EditRejection struct {
    Index  int    `json:"index"`
    Reason string `json:"reason"`
}

type EditResult struct {
    EditsApplied  []Edit
    EditsRejected []EditRejection
    NewSections   model.SectionMap
}

type Tailor interface {
    // ApplyEdits applies the edit envelope to the given sections and returns the
    // new sections along with per-edit success/failure tracking.
    //
    // Edits are applied in order; each is independent (one rejection does not
    // abort subsequent edits). The input sections are not mutated.
    ApplyEdits(ctx context.Context, sections model.SectionMap, edits []Edit) (EditResult, error)
}
```

**Adapter** (`internal/service/tailor/`):
- Single implementation backs both T1 and T2 tool handlers.
- Stateless; no LLM required for any edit op (the orchestrator already decided the edits).
- Validation errors within an edit produce `EditRejection`; a non-nil error return is reserved for structural failures (nil sections, unrecoverable schema corruption).

**Deprecated in this feature (removed at end):**
- `TailorResume(ctx, *model.TailorInput) (TailorResult, error)` — superseded. The pipeline's T1/T2 callers switch to `ApplyEdits`.

---

## 6. Compile-time interface guards

Every adapter package MUST include the standard assertion pattern:

```go
var _ port.Renderer = (*Service)(nil)
var _ port.Extractor = (*IdentityService)(nil)
var _ port.Tailor = (*Service)(nil)
var _ port.ResumeRepository = (*ResumeRepository)(nil)
var _ port.Orchestrator = (*Service)(nil)
```

---

## 7. Error types exposed via ports

Defined in `internal/model/errors.go`:

| Symbol | Meaning | Caller response |
|---|---|---|
| `ErrSectionsMissing` | Sections sidecar absent | MCP: typed envelope; Headless: trigger `Orchestrator.ParseSections` |
| `ErrSchemaVersionUnsupported` | Persisted sections schema > current | MCP: typed envelope; Headless: error out |
| `ErrNotSupportedInMCPMode` | Orchestrator method not implemented in MCP stub | Handler treats as internal error |
| `SchemaError{Field, Reason}` | Section validation failed | Propagated to caller verbatim |

All errors are wrapped at boundaries per constitution: `fmt.Errorf("context: %w", err)`.
