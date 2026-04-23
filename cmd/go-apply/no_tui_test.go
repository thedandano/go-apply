package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoTUIReferences enforces FR-015: go-apply must never import TUI packages.
// charmbracelet/log (structured logging) is explicitly allowed; only the
// interactive UI packages below are prohibited.
var tuiPatterns = []string{
	"charmbracelet/bubbletea",
	"charmbracelet/lipgloss",
	"charmbracelet/bubbles",
	"tea.Program",
}

func TestNoTUIReferences(t *testing.T) {
	root := filepath.Join("..", "..")
	var violations []string

	for _, dir := range []string{"internal", "cmd"} {
		target := filepath.Join(root, dir)
		err := filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, pat := range tuiPatterns {
				if strings.Contains(string(content), pat) {
					rel, _ := filepath.Rel(root, path)
					violations = append(violations, rel+": contains "+pat)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", target, err)
		}
	}

	if len(violations) > 0 {
		t.Errorf("FR-015: TUI package references found (charmbracelet/log is allowed; bubbletea/lipgloss/bubbles are not):\n  %s",
			strings.Join(violations, "\n  "))
	}
}
