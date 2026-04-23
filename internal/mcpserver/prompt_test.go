package mcpserver

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// allowedHosts is the closed set of hosts permitted in the composed prompt.
// Built from an audit of the vendored skills/resume-tailor.md — only
// yoursite.com appears (placeholder in template_generator.py CLI example).
var allowedHosts = map[string]bool{
	"yoursite.com": true,
}

// urlPattern extracts URLs without consuming trailing punctuation that may
// follow them in markdown contexts (quotes, parens, backticks, brackets).
var urlPattern = regexp.MustCompile(`https?://[^\s"'<>)\]` + "`" + `]+`)

func TestTailorSkillBodyByteCount(t *testing.T) {
	if len(tailorSkillBody) <= 2000 {
		t.Errorf("tailorSkillBody is suspiciously small: %d bytes (expected > 2000)", len(tailorSkillBody))
	}
}

func TestTailorSkillBodyIntegrityHash(t *testing.T) {
	// Locate the .sha256 sentinel relative to this test file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	hashFile := filepath.Join(filepath.Dir(thisFile), "skills", "resume-tailor.md.sha256")

	raw, err := os.ReadFile(hashFile)
	if err != nil {
		t.Fatalf("cannot read hash sentinel %s: %v — did you run `make sync-skill`?", hashFile, err)
	}
	want := strings.TrimSpace(string(raw))

	got := fmt.Sprintf("%x", sha256.Sum256([]byte(tailorSkillBody)))
	if got != want {
		t.Errorf("embedded body does not match checked-in .sha256 — re-run `make sync-skill`\n  want: %s\n  got:  %s", want, got)
	}
}

func TestTailorResumePromptPreludePositiveMarkers(t *testing.T) {
	lower := strings.ToLower(tailorResumePromptText)

	positives := []struct {
		name  string
		check func() bool
	}{
		{
			name:  "submit_tailored_resume (exact)",
			check: func() bool { return strings.Contains(tailorResumePromptText, "submit_tailored_resume") },
		},
		{
			name:  "get_config (exact)",
			check: func() bool { return strings.Contains(tailorResumePromptText, "get_config") },
		},
		{
			name:  "do not invoke (case-insensitive)",
			check: func() bool { return strings.Contains(lower, "do not invoke") },
		},
		{
			name:  "not part of go-apply (case-insensitive)",
			check: func() bool { return strings.Contains(lower, "not part of go-apply") },
		},
		{
			name:  "resume-tailor and separately co-located within 200 chars",
			check: func() bool { return resumeTailorAndSeparatelyColocated(tailorResumePromptText) },
		},
		{
			name:  "do not produce a modification_spec (case-insensitive)",
			check: func() bool { return strings.Contains(lower, "do not produce a modification_spec") },
		},
		{
			name:  "resume_modifier.py (exact)",
			check: func() bool { return strings.Contains(tailorResumePromptText, "resume_modifier.py") },
		},
	}

	for _, tc := range positives {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if !tc.check() {
				t.Errorf("composed prompt is missing required marker: %q", tc.name)
			}
		})
	}
}

func TestTailorResumePromptPreludeNegativeMarkers(t *testing.T) {
	lower := strings.ToLower(tailorResumePromptText)

	negatives := []string{
		"future release",
		"future change",
		"deferred",
		"phase 2",
		"phase ii",
	}

	for _, marker := range negatives {
		marker := marker
		t.Run("must_not_contain/"+marker, func(t *testing.T) {
			if strings.Contains(lower, marker) {
				t.Errorf("composed prompt must NOT contain %q but it does", marker)
			}
		})
	}
}

func TestTailorResumePromptURLAllowlist(t *testing.T) {
	matches := urlPattern.FindAllString(tailorResumePromptText, -1)
	for _, raw := range matches {
		u, err := url.Parse(raw)
		if err != nil {
			t.Errorf("cannot parse extracted URL %q: %v", raw, err)
			continue
		}
		host := u.Hostname()
		if !allowedHosts[host] {
			t.Errorf("URL host %q (from %q) is not in the allowlist — add it if legitimate", host, raw)
		}
	}
}

func TestTailorSkillBodyMarkers(t *testing.T) {
	lower := strings.ToLower(tailorSkillBody)

	markers := []struct {
		name  string
		check func() bool
	}{
		{
			name:  "Golden Rule (case-insensitive)",
			check: func() bool { return strings.Contains(lower, "golden rule") },
		},
		{
			name:  "accomplishments",
			check: func() bool { return strings.Contains(tailorSkillBody, "accomplishments") },
		},
		{
			name: "skills_reference or skills reference",
			check: func() bool {
				return strings.Contains(tailorSkillBody, "skills_reference") || strings.Contains(lower, "skills reference")
			},
		},
		{
			name:  "existing (resume bullets)",
			check: func() bool { return strings.Contains(lower, "existing") },
		},
		{
			name:  "Tier 1",
			check: func() bool { return strings.Contains(tailorSkillBody, "Tier 1") },
		},
		{
			name:  "Tier 2",
			check: func() bool { return strings.Contains(tailorSkillBody, "Tier 2") },
		},
		{
			name:  "Voice and Honesty (case-insensitive)",
			check: func() bool { return strings.Contains(lower, "voice and honesty") },
		},
	}

	for _, tc := range markers {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if !tc.check() {
				t.Errorf("skill body is missing required marker: %q", tc.name)
			}
		})
	}
}

// resumeTailorAndSeparatelyColocated returns true if "resume-tailor" and
// "separately" appear within 200 characters of each other in the text.
func resumeTailorAndSeparatelyColocated(text string) bool {
	lower := strings.ToLower(text)
	needle := "resume-tailor"
	for i := 0; i < len(lower); i++ {
		idx := strings.Index(lower[i:], needle)
		if idx < 0 {
			break
		}
		abs := i + idx
		// search a window of 200 chars before and after
		start := abs - 200
		if start < 0 {
			start = 0
		}
		end := abs + len(needle) + 200
		if end > len(lower) {
			end = len(lower)
		}
		if strings.Contains(lower[start:end], "separately") {
			return true
		}
		i = abs + 1
	}
	return false
}
