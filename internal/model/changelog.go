package model

import "fmt"

// ChangelogKind names the category of change the LLM made.
type ChangelogKind string

const (
	ChangelogSkillAdd      ChangelogKind = "skill_add"
	ChangelogBulletRewrite ChangelogKind = "bullet_rewrite"
	ChangelogSkip          ChangelogKind = "skip"
	ChangelogSummaryUpdate ChangelogKind = "summary_update"
)

// ChangelogTier names the /resume-tailor skill tier the change came from.
type ChangelogTier string

const (
	ChangelogTier1   ChangelogTier = "tier_1"
	ChangelogTier2   ChangelogTier = "tier_2"
	ChangelogSummary ChangelogTier = "summary"
)

// ChangelogRewriteSource names where a bullet rewrite's content came from.
type ChangelogRewriteSource string

const (
	RewriteSourceAccomplishmentsDoc    ChangelogRewriteSource = "accomplishments_doc"
	RewriteSourceExistingBulletReframe ChangelogRewriteSource = "existing_bullet_reframe"
)

// ChangelogSkipReason names why the LLM skipped a target keyword.
type ChangelogSkipReason string

const (
	SkipReasonNotInSkillsReference ChangelogSkipReason = "not_in_skills_reference"
	SkipReasonScopeCheckFailed     ChangelogSkipReason = "scope_check_failed"
	SkipReasonNoBasisFound         ChangelogSkipReason = "no_basis_found"
)

// ChangelogEntry is a single typed record of what the LLM-driven tailor did.
// Kind drives which payload fields are meaningful; all other payload fields
// must be zero-valued for a given entry.
type ChangelogEntry struct {
	Kind    ChangelogKind `json:"kind"`
	Tier    ChangelogTier `json:"tier"`
	Keyword string        `json:"keyword,omitempty"` // empty for summary_update

	// skill_add payload
	Subsection string `json:"subsection,omitempty"`

	// bullet_rewrite payload
	Role   string                 `json:"role,omitempty"`
	Before string                 `json:"before,omitempty"`
	After  string                 `json:"after,omitempty"`
	Source ChangelogRewriteSource `json:"source,omitempty"`

	// skip payload
	Reason ChangelogSkipReason `json:"reason,omitempty"`

	// summary_update payload
	Note string `json:"note,omitempty"`
}

// ValidateChangelogEntry checks that e is a well-formed entry: known Kind and
// Tier, length caps satisfied, required payload fields present, and extraneous
// payload fields absent. Returns an error prefixed with "invalid_changelog: "
// on any violation. Note absence for summary_update is a warn-not-reject rule
// and is NOT checked here; callers that need the warning must check separately.
func ValidateChangelogEntry(e *ChangelogEntry) error {
	switch e.Kind {
	case ChangelogSkillAdd, ChangelogBulletRewrite, ChangelogSkip, ChangelogSummaryUpdate:
	default:
		return fmt.Errorf("invalid_changelog: Kind %q is not a recognised ChangelogKind", e.Kind)
	}

	switch e.Tier {
	case ChangelogTier1, ChangelogTier2, ChangelogSummary:
	default:
		return fmt.Errorf("invalid_changelog: Tier %q is not a recognised ChangelogTier", e.Tier)
	}

	// Length caps applied before per-kind checks.
	if len(e.Keyword) > 128 {
		return fmt.Errorf("invalid_changelog: Keyword length %d exceeds cap of 128", len(e.Keyword))
	}
	if len(e.Subsection) > 64 {
		return fmt.Errorf("invalid_changelog: Subsection length %d exceeds cap of 64", len(e.Subsection))
	}
	if len(e.Before) > 2000 {
		return fmt.Errorf("invalid_changelog: Before length %d exceeds cap of 2000", len(e.Before))
	}
	if len(e.After) > 2000 {
		return fmt.Errorf("invalid_changelog: After length %d exceeds cap of 2000", len(e.After))
	}
	if len(e.Note) > 2000 {
		return fmt.Errorf("invalid_changelog: Note length %d exceeds cap of 2000", len(e.Note))
	}

	switch e.Kind {
	case ChangelogSkillAdd:
		// Allowed: Keyword, Subsection. All other payload fields must be empty.
		if e.Role != "" {
			return fmt.Errorf("invalid_changelog: Role %q must be empty for kind %q", e.Role, e.Kind)
		}
		if e.Before != "" {
			return fmt.Errorf("invalid_changelog: Before must be empty for kind %q", e.Kind)
		}
		if e.After != "" {
			return fmt.Errorf("invalid_changelog: After must be empty for kind %q", e.Kind)
		}
		if e.Source != "" {
			return fmt.Errorf("invalid_changelog: Source %q must be empty for kind %q", e.Source, e.Kind)
		}
		if e.Reason != "" {
			return fmt.Errorf("invalid_changelog: Reason %q must be empty for kind %q", e.Reason, e.Kind)
		}
		if e.Note != "" {
			return fmt.Errorf("invalid_changelog: Note must be empty for kind %q", e.Kind)
		}

	case ChangelogBulletRewrite:
		// Required: Before, After, Source (valid). Allowed: Keyword, Role.
		// Excluded: Subsection, Reason, Note.
		if e.Before == "" {
			return fmt.Errorf("invalid_changelog: Before is required for kind %q", e.Kind)
		}
		if e.After == "" {
			return fmt.Errorf("invalid_changelog: After is required for kind %q", e.Kind)
		}
		switch e.Source {
		case RewriteSourceAccomplishmentsDoc, RewriteSourceExistingBulletReframe:
		default:
			return fmt.Errorf("invalid_changelog: Source %q is not a recognised ChangelogRewriteSource for kind %q", e.Source, e.Kind)
		}
		if e.Subsection != "" {
			return fmt.Errorf("invalid_changelog: Subsection %q must be empty for kind %q", e.Subsection, e.Kind)
		}
		if e.Reason != "" {
			return fmt.Errorf("invalid_changelog: Reason %q must be empty for kind %q", e.Reason, e.Kind)
		}
		if e.Note != "" {
			return fmt.Errorf("invalid_changelog: Note must be empty for kind %q", e.Kind)
		}

	case ChangelogSkip:
		// Required: Reason (valid). Allowed: Keyword.
		// Excluded: Subsection, Role, Before, After, Source, Note.
		switch e.Reason {
		case SkipReasonNotInSkillsReference, SkipReasonScopeCheckFailed, SkipReasonNoBasisFound:
		default:
			return fmt.Errorf("invalid_changelog: Reason %q is not a recognised ChangelogSkipReason for kind %q", e.Reason, e.Kind)
		}
		if e.Subsection != "" {
			return fmt.Errorf("invalid_changelog: Subsection %q must be empty for kind %q", e.Subsection, e.Kind)
		}
		if e.Role != "" {
			return fmt.Errorf("invalid_changelog: Role %q must be empty for kind %q", e.Role, e.Kind)
		}
		if e.Before != "" {
			return fmt.Errorf("invalid_changelog: Before must be empty for kind %q", e.Kind)
		}
		if e.After != "" {
			return fmt.Errorf("invalid_changelog: After must be empty for kind %q", e.Kind)
		}
		if e.Source != "" {
			return fmt.Errorf("invalid_changelog: Source %q must be empty for kind %q", e.Source, e.Kind)
		}
		if e.Note != "" {
			return fmt.Errorf("invalid_changelog: Note must be empty for kind %q", e.Kind)
		}

	case ChangelogSummaryUpdate:
		// Keyword MUST be empty. Allowed: Note (empty is warn-not-reject).
		// Excluded: Subsection, Role, Before, After, Source, Reason.
		if e.Keyword != "" {
			return fmt.Errorf("invalid_changelog: Keyword %q must be empty for kind %q", e.Keyword, e.Kind)
		}
		if e.Subsection != "" {
			return fmt.Errorf("invalid_changelog: Subsection %q must be empty for kind %q", e.Subsection, e.Kind)
		}
		if e.Role != "" {
			return fmt.Errorf("invalid_changelog: Role %q must be empty for kind %q", e.Role, e.Kind)
		}
		if e.Before != "" {
			return fmt.Errorf("invalid_changelog: Before must be empty for kind %q", e.Kind)
		}
		if e.After != "" {
			return fmt.Errorf("invalid_changelog: After must be empty for kind %q", e.Kind)
		}
		if e.Source != "" {
			return fmt.Errorf("invalid_changelog: Source %q must be empty for kind %q", e.Source, e.Kind)
		}
		if e.Reason != "" {
			return fmt.Errorf("invalid_changelog: Reason %q must be empty for kind %q", e.Reason, e.Kind)
		}
	}

	return nil
}
