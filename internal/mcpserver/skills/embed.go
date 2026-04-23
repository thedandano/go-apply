// Package skills exposes prompts and other resources that the MCP server
// ships to its host agent. Prompts are vendored from external sources and
// embedded at build time so the binary is self-contained.
package skills

import (
	_ "embed"
	"strings"
)

//go:embed resume-tailor.md
var resumeTailorPromptBody string

// PromptBody returns the embedded /resume-tailor skill body. The body is
// concatenated with a runtime header by the tailor adapter to produce the
// final prompt_bundle returned from tailor_begin.
//
// The embedded body MUST be non-empty; an empty body indicates a broken
// build (the vendored SKILL.md is missing or empty) and callers MUST NOT
// proceed — see MustBeLoaded for the startup guard.
func PromptBody() string {
	return resumeTailorPromptBody
}

// MustBeLoaded panics if the embedded prompt body is empty or whitespace-only.
// Call once at process startup so a broken build fails before any tailor
// session is opened.
func MustBeLoaded() {
	if strings.TrimSpace(resumeTailorPromptBody) == "" {
		panic("mcpserver/skills: embedded resume-tailor.md is empty; run `make sync-tailor-prompt`")
	}
}
