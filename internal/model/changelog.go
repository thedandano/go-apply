package model

// ChangelogEntry records a single agent-driven tailoring action applied to a resume.
// The Action and Target fields are constrained to the allowed value sets enforced by
// ValidateChangelogAction and ValidateChangelogTarget. Length caps (keyword ≤ 128 bytes,
// reason ≤ 512 bytes) are enforced by the handler that accepts agent input (Unit 3).
type ChangelogEntry struct {
	Action  string `json:"action"`
	Target  string `json:"target"`
	Keyword string `json:"keyword"`
	Reason  string `json:"reason,omitempty"`
}

// ValidateChangelogAction returns true if and only if a is one of the allowed action values.
// The comparison is case-sensitive; "Added" is rejected, "added" is accepted.
func ValidateChangelogAction(a string) bool {
	switch a {
	case "added", "rewrote", "skipped":
		return true
	}
	return false
}

// ValidateChangelogTarget returns true if and only if t is one of the allowed target values.
// The comparison is case-sensitive; "Skill" is rejected, "skill" is accepted.
func ValidateChangelogTarget(t string) bool {
	switch t {
	case "skill", "bullet", "summary":
		return true
	}
	return false
}
