package tailor

import (
	"strings"
	"testing"
)

func TestIsHeaderLine(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Markdown headers — always a section header.
		{"## Skills", true},
		{"# Experience", true},

		// Known single-word section keyword (all-caps).
		{"SKILLS", true},

		// Colon-terminated section titles.
		{"Skills:", true},
		{"EXPERIENCE:", true},

		// Known compound headers (all-caps variants).
		{"WORK EXPERIENCE", true},
		{"TECHNICAL SKILLS", true},
		{"PROFESSIONAL EXPERIENCE", true},

		// Company names — not a known keyword.
		{"ACME CORP", false},

		// Abbreviated date — starts with a month abbreviation, not a known keyword.
		{"JAN 2020", false},

		// Bullet lines are never headers.
		{"• Python, Go, Kubernetes", false},
		{"- Led a team", false},
		{"* Delivered features", false},

		// Prose line.
		{"Developed scalable APIs for high-traffic systems", false},

		// Date range.
		{"2018 – 2022", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			got := isHeaderLine(tc.input)
			if got != tc.want {
				t.Errorf("isHeaderLine(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestAddKeywords_SkipPresent(t *testing.T) {
	resume := `# Experience
- Built Kubernetes clusters

## Skills
Python, Golang, Docker
`
	// Docker is already in Skills — should not be re-added.
	modified, added := AddKeywordsToSkillsSection(resume, []string{"Docker", "Rust"})

	for _, kw := range added {
		if strings.EqualFold(kw, "Docker") {
			t.Error("Docker was already present but was reported as added")
		}
	}
	if len(added) != 1 || added[0] != "Rust" {
		t.Errorf("expected only Rust to be added, got %v", added)
	}
	if !strings.Contains(modified, "Rust") {
		t.Error("modified resume does not contain the newly added keyword Rust")
	}
}

func TestAddKeywords_NoSkillsSection(t *testing.T) {
	resume := `# Experience
- Built Kubernetes clusters
`
	modified, added := AddKeywordsToSkillsSection(resume, []string{"Python", "Golang"})

	if modified != resume {
		t.Error("resume should be unchanged when Skills section is absent")
	}
	if len(added) != 0 {
		t.Errorf("expected no keywords added, got %v", added)
	}
}

func TestAddKeywords_InjectsAll(t *testing.T) {
	resume := `## Skills
Python
`
	_, added := AddKeywordsToSkillsSection(resume, []string{"Golang", "Docker"})

	if len(added) != 2 {
		t.Errorf("expected 2 keywords added, got %v", added)
	}
}

func TestAddKeywords_CaseInsensitiveDedup(t *testing.T) {
	resume := `## Skills
python, GOLANG
`
	// Supply mixed-case variants — none should be re-added.
	_, added := AddKeywordsToSkillsSection(resume, []string{"Python", "golang", "PYTHON"})

	if len(added) != 0 {
		t.Errorf("expected no keywords added (all already present case-insensitively), got %v", added)
	}
}

func TestAddKeywords_TechnicalSkillsHeader(t *testing.T) {
	resume := `# Experience
- Built distributed systems

## Technical Skills
Python, Docker
`
	modified, added := AddKeywordsToSkillsSection(resume, []string{"Golang", "Kubernetes"})

	if len(added) != 2 {
		t.Errorf("expected 2 keywords added under '## Technical Skills', got %v", added)
	}
	if !strings.Contains(modified, "Golang") {
		t.Error("modified resume does not contain injected keyword Golang")
	}
	if !strings.Contains(modified, "Kubernetes") {
		t.Error("modified resume does not contain injected keyword Kubernetes")
	}
}
