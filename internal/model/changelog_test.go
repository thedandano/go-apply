package model

import "testing"

func TestValidateChangelogAction(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		// Allowed values.
		{"added", "added", true},
		{"rewrote", "rewrote", true},
		{"skipped", "skipped", true},
		// Rejected values.
		{"unknown", "replaced", false},
		{"empty", "", false},
		// Case sensitivity — uppercase variants must be rejected.
		{"Added_uppercase", "Added", false},
		{"Rewrote_uppercase", "Rewrote", false},
		{"Skipped_uppercase", "Skipped", false},
		{"ADDED_allcaps", "ADDED", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ValidateChangelogAction(c.input)
			if got != c.want {
				t.Errorf("ValidateChangelogAction(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}

func TestValidateChangelogTarget(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		// Allowed values.
		{"skill", "skill", true},
		{"bullet", "bullet", true},
		{"summary", "summary", true},
		// Rejected values.
		{"unknown", "header", false},
		{"empty", "", false},
		// Case sensitivity — uppercase variants must be rejected.
		{"Skill_uppercase", "Skill", false},
		{"Bullet_uppercase", "Bullet", false},
		{"Summary_uppercase", "Summary", false},
		{"SKILL_allcaps", "SKILL", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ValidateChangelogTarget(c.input)
			if got != c.want {
				t.Errorf("ValidateChangelogTarget(%q) = %v, want %v", c.input, got, c.want)
			}
		})
	}
}
