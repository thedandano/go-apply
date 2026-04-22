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
	modified, added, found := AddKeywordsToSkillsSection(resume, []string{"Docker", "Rust"})

	if !found {
		t.Error("expected skills section to be found")
	}
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
	modified, added, found := AddKeywordsToSkillsSection(resume, []string{"Python", "Golang"})

	if found {
		t.Error("expected skills section NOT found; caller must be able to distinguish 'no section' from 'all present'")
	}
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
	_, added, found := AddKeywordsToSkillsSection(resume, []string{"Golang", "Docker"})

	if !found {
		t.Error("expected skills section to be found")
	}
	if len(added) != 2 {
		t.Errorf("expected 2 keywords added, got %v", added)
	}
}

func TestAddKeywords_CaseInsensitiveDedup(t *testing.T) {
	resume := `## Skills
python, GOLANG
`
	// Supply mixed-case variants — none should be re-added.
	_, added, found := AddKeywordsToSkillsSection(resume, []string{"Python", "golang", "PYTHON"})

	if !found {
		t.Error("expected skills section to be found even when no keywords are added")
	}
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
	modified, added, found := AddKeywordsToSkillsSection(resume, []string{"Golang", "Kubernetes"})

	if !found {
		t.Error("expected '## Technical Skills' to be detected as a skills section")
	}
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

// TestAddKeywords_SkillsWithTrailingText covers header variants where the line
// doesn't end immediately after "SKILLS" — the common PDF-extracted forms
// "SKILLS & ABILITIES" and "SKILLS AND ABILITIES". Regression test for the
// regex-anchored detection bug.
func TestAddKeywords_SkillsWithTrailingText(t *testing.T) {
	variants := []string{
		"SKILLS & ABILITIES",
		"SKILLS AND ABILITIES",
		"Skills & Abilities",
		"Key Skills & Competencies",
	}
	for _, header := range variants {
		header := header
		t.Run(header, func(t *testing.T) {
			resume := header + "\nPython, SQL\n\nPROFESSIONAL EXPERIENCE\n- Built systems\n"
			modified, added, found := AddKeywordsToSkillsSection(resume, []string{"Golang"})

			if !found {
				t.Errorf("expected %q to be detected as a skills section", header)
			}
			if len(added) != 1 || added[0] != "Golang" {
				t.Errorf("expected Golang to be added under %q, got %v", header, added)
			}
			if !strings.Contains(modified, "Golang") {
				t.Errorf("modified resume missing injected keyword Golang under %q", header)
			}
		})
	}
}

// TestAddKeywords_NonSkillsHeaderNotMatched guards against over-matching.
// Headers that don't mention skills must not be treated as the skills section.
func TestAddKeywords_NonSkillsHeaderNotMatched(t *testing.T) {
	resume := `PROFESSIONAL EXPERIENCE
- Built systems

EDUCATION
- BS Computer Science
`
	_, added, found := AddKeywordsToSkillsSection(resume, []string{"Golang"})

	if found {
		t.Error("no header in this resume mentions 'skills' — section should not be found")
	}
	if len(added) != 0 {
		t.Errorf("expected no keywords added when no skills section exists, got %v", added)
	}
}
