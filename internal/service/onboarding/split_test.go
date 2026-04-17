package onboarding

import (
	"testing"
)

func TestSplitAccomplishmentSections(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "markdown ## headings — two sections",
			input: "## Scaled backend\nLed team of 5 engineers\n\n## Reduced latency\nCut p99 from 800ms to 120ms",
			want:  2,
		},
		{
			name:  "markdown # heading — three sections",
			input: "# SDE 1\n## Led migration\nFoo\n## Launched product\nBar",
			want:  3,
		},
		{
			name:  "preamble before first heading is its own chunk",
			input: "Intro paragraph\n\n## First achievement\nDetails here",
			want:  2,
		},
		{
			name:  "plain-text STAR format — two accomplishments separated by blank line",
			input: "Onboarding Weight Data\nSituation: Foo\nBehavior: Bar\nImpact: Baz\n\nPost Incident Review\nSituation: A\nBehavior: B\nImpact: C",
			want:  2,
		},
		{
			name:  "plain-text STAR format — three accomplishments",
			input: "Accomplishment One\nSituation: S\nBehavior: B\nImpact: I\n\nAccomplishment Two\nSituation: S\nBehavior: B\nImpact: I\n\nAccomplishment Three\nSituation: S\nBehavior: B\nImpact: I",
			want:  3,
		},
		{
			name:  "no headings single paragraph falls back to one chunk",
			input: "Some free-form accomplishment text with no headings.",
			want:  1,
		},
		{
			name:  "empty text returns no chunks",
			input: "",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitAccomplishmentSections(tt.input)
			if len(got) != tt.want {
				t.Errorf("splitAccomplishmentSections: got %d chunks, want %d\nchunks: %v", len(got), tt.want, got)
			}
		})
	}
}

func TestCountSkillItems(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{
			name:  "comma-separated on one line",
			input: "Go, Python, Docker",
			want:  3,
		},
		{
			name:  "one skill per line",
			input: "Go\nPython\nDocker",
			want:  3,
		},
		{
			name:  "mixed: headings skipped commas counted",
			input: "# Languages\nGo, Python\n# Tools\nDocker, Kubernetes",
			want:  4,
		},
		{
			name:  "blank lines ignored",
			input: "Go, Python\n\nDocker",
			want:  3,
		},
		{
			name:  "empty text",
			input: "",
			want:  0,
		},
		{
			name:  "heading only",
			input: "# Skills",
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countSkillItems(tt.input)
			if got != tt.want {
				t.Errorf("countSkillItems(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
