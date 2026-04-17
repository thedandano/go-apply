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
			name:  "two sections",
			input: "## Scaled backend\nLed team of 5 engineers\n\n## Reduced latency\nCut p99 from 800ms to 120ms",
			want:  2,
		},
		{
			name:  "no headings falls back to single chunk",
			input: "Some free-form accomplishment text with no headings.",
			want:  1,
		},
		{
			name:  "three sections",
			input: "## Led migration\nFoo\n## Launched product\nBar\n## Hired team\nBaz",
			want:  3,
		},
		{
			name:  "empty text returns no chunks",
			input: "",
			want:  0,
		},
		{
			name:  "preamble before first heading is its own chunk",
			input: "Intro paragraph\n\n## First achievement\nDetails here",
			want:  2,
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
