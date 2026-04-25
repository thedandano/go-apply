package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// EditOp is the set of allowed edit operations.
type EditOp string

const (
	EditOpAdd     EditOp = "add"
	EditOpRemove  EditOp = "remove"
	EditOpReplace EditOp = "replace"
)

// Edit is a single mutation instruction in the unified edit envelope.
// Target is required for "replace"/"remove" on experience bullets (exp-<i>-b<j>).
// Value is required for "add"/"replace".
type Edit struct {
	Section  string `json:"section"`
	Op       EditOp `json:"op"`
	Target   string `json:"target,omitempty"`
	Value    string `json:"value,omitempty"`
	Category string `json:"category,omitempty"`
}

// EditRejection records a rejected edit with its reason.
type EditRejection struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

// EditResult is the response from ApplyEdits.
type EditResult struct {
	EditsApplied  []Edit           `json:"edits_applied"`
	EditsRejected []EditRejection  `json:"edits_rejected"`
	NewSections   model.SectionMap `json:"new_sections"`
}

// Tailor rewrites a resume to better match a job description.
// The pipeline drives the tier loop; TailorResume executes a single tier pass.
type Tailor interface {
	TailorResume(ctx context.Context, input *model.TailorInput) (model.TailorResult, error)
	// ApplyEdits applies the edit envelope to the given sections and returns the
	// new sections along with per-edit success/failure tracking.
	//
	// Edits are applied in order; each is independent (one rejection does not
	// abort subsequent edits). The input sections are not mutated.
	ApplyEdits(ctx context.Context, sections model.SectionMap, edits []Edit) (EditResult, error)
}
