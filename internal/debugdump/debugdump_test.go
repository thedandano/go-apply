package debugdump

import (
	"strings"
	"testing"
)

func TestDiffText_IdenticalInputs(t *testing.T) {
	t.Parallel()
	result := DiffText("test", "same content", "same content")
	if result != "" {
		t.Errorf("expected empty string for identical inputs, got: %q", result)
	}
}

func TestDiffText_DifferentInputs(t *testing.T) {
	t.Parallel()
	before := "line one\nline two\n"
	after := "line one\nline two modified\n"
	result := DiffText("test", before, after)
	if result == "" {
		t.Error("expected non-empty unified diff for different inputs")
	}
	if !strings.Contains(result, "line two modified") {
		t.Errorf("expected diff to contain changed content, got: %q", result)
	}
}

func TestDiffSection_ExtractsAndDiffsSkillsSection(t *testing.T) {
	t.Parallel()
	before := `## Summary
Some summary text.

## Skills
Go, Python, SQL

## Experience
Some experience.`

	after := `## Summary
Some summary text.

## Skills
Go, Python, SQL, Docker

## Experience
Some experience.`

	result := DiffSection("test", "Skills", before, after)
	if result == "" {
		t.Error("expected non-empty diff for changed Skills section")
	}
	if strings.Contains(result, "Experience") {
		t.Errorf("diff should not contain Experience section content, got: %q", result)
	}
	if !strings.Contains(result, "Docker") {
		t.Errorf("expected diff to contain added keyword 'Docker', got: %q", result)
	}
}

func TestDiffSection_FallsBackWhenSectionNotFound(t *testing.T) {
	t.Parallel()
	before := "## Summary\nSome summary.\n\n## Experience\nSome experience."
	after := "## Summary\nChanged summary.\n\n## Experience\nSome experience."

	// "Skills" section is absent in both — should fall back to full diff
	result := DiffSection("test", "Skills", before, after)
	if result == "" {
		t.Error("expected fallback full diff when section not found in either string")
	}
	if !strings.Contains(result, "Changed summary") {
		t.Errorf("expected full diff to contain changed content, got: %q", result)
	}
}

func TestDump_ReturnsNonEmptyStringWithLabel(t *testing.T) {
	t.Parallel()
	type sample struct {
		Name  string
		Value int
	}
	result := Dump("myLabel", sample{Name: "foo", Value: 42})
	if result == "" {
		t.Error("expected non-empty string from Dump")
	}
	if !strings.HasPrefix(result, "myLabel: ") {
		t.Errorf("expected result to start with label, got: %q", result)
	}
	if !strings.Contains(result, "foo") {
		t.Errorf("expected result to contain struct field value 'foo', got: %q", result)
	}
}
