package debugdump

import (
	"fmt"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	pp "github.com/k0kubun/pp/v3"
)

// printer is a package-level pp instance with colors disabled for log safety.
var printer = func() *pp.PrettyPrinter {
	p := pp.New()
	p.SetColoringEnabled(false)
	return p
}()

// Dump returns a pretty-printed representation of v with colors disabled.
func Dump(label string, v any) string {
	return label + ": " + printer.Sprint(v)
}

// DiffText returns a unified diff of before and after.
// Returns an empty string when the two strings are identical.
func DiffText(label, before, after string) string {
	edits := myers.ComputeEdits(span.URIFromPath(label), before, after)
	diff := gotextdiff.ToUnified(label+":before", label+":after", before, edits)
	unified := fmt.Sprint(diff)
	if strings.TrimSpace(unified) == "" {
		return ""
	}
	return unified
}

// DiffSection extracts the named section from before and after, then returns
// a unified diff of just that section. Falls back to a full DiffText when the
// section header is not found in either string.
// Section detection: scans for a line whose trimmed, lowercased content equals
// strings.ToLower(section) (with optional leading '#' stripped and trailing ':' stripped).
func DiffSection(label, section, before, after string) string {
	beforeSec := extractSection(before, section)
	afterSec := extractSection(after, section)
	if beforeSec == "" && afterSec == "" {
		return DiffText(label, before, after)
	}
	return DiffText(label+"."+strings.ToLower(section), beforeSec, afterSec)
}

// extractSection finds the named section in text and returns its lines (from
// the header line up to but not including the next header line).
// Returns "" if the section is not found.
func extractSection(text, section string) string {
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if isSectionHeader(line, section) {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if isAnyHeader(lines[i]) {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

// isSectionHeader reports whether line is a header for the given section name.
func isSectionHeader(line, section string) bool {
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)
	lower = strings.TrimLeft(lower, "#")
	lower = strings.TrimSpace(lower)
	lower = strings.TrimRight(lower, ":")
	lower = strings.TrimSpace(lower)
	return strings.EqualFold(lower, section)
}

// isAnyHeader reports whether line looks like a section header.
func isAnyHeader(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 {
		return false
	}
	// Markdown heading
	if strings.HasPrefix(trimmed, "#") {
		return true
	}
	// ALL CAPS line (common in plain-text resumes)
	upper := strings.ToUpper(trimmed)
	return upper == trimmed && len(trimmed) >= 3 && !strings.ContainsAny(trimmed, "•-*")
}
