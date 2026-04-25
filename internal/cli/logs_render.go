package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// quotedFieldRe matches logfmt key="value" pairs where the value is a quoted string.
// Key characters include letters, digits, underscores, hyphens, and dots.
var quotedFieldRe = regexp.MustCompile(`([\w.-]+)=("[^"\\]*(?:\\.[^"\\]*)*")`)

// jsonField holds a key and its parsed JSON value extracted from a log line.
type jsonField struct {
	key    string
	parsed json.RawMessage
}

// renderLine writes a pretty-printed representation of a logfmt log line to w.
// Fields whose quoted string values are valid JSON objects or arrays are removed
// from the header line and printed below it, indented, under a "key:" label.
// Lines containing no JSON-valued fields are written unchanged.
// Errors from w are returned.
func renderLine(w io.Writer, line string) error {
	matches := quotedFieldRe.FindAllStringSubmatchIndex(line, -1)

	fields := make([]jsonField, 0, len(matches))
	removedRanges := make([][2]int, 0, len(matches))

	for _, m := range matches {
		// m[0]:m[1] = full match, m[2]:m[3] = key, m[4]:m[5] = quoted value
		key := line[m[2]:m[3]]
		rawValue := line[m[4]:m[5]]

		unquoted, ok := logfmtUnquote(rawValue)
		if !ok {
			continue
		}

		var raw json.RawMessage
		if err := json.Unmarshal([]byte(unquoted), &raw); err != nil {
			continue
		}

		firstByte := unquoted[0]
		if firstByte != '{' && firstByte != '[' {
			// JSON primitive — stays on header
			continue
		}

		fields = append(fields, jsonField{key: key, parsed: raw})
		// Consume the trailing space after the match (if any) to avoid
		// double-spaces in the header when the field is removed from
		// the middle of the line.
		end := m[1]
		if end < len(line) && line[end] == ' ' {
			end++
		}
		removedRanges = append(removedRanges, [2]int{m[0], end})
	}

	// Build header by excising JSON field matches; trim any trailing whitespace
	// left by removing a terminal field.
	header := strings.TrimRight(removeRanges(line, removedRanges), " \t")

	if _, err := fmt.Fprintf(w, "%s\n", header); err != nil {
		return err
	}

	for _, f := range fields {
		indented, err := json.MarshalIndent(f.parsed, "    ", "  ")
		if err != nil {
			// Should not happen since we already parsed it; skip gracefully.
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s:\n    %s\n", f.key, indented); err != nil {
			return err
		}
	}

	return nil
}

// logfmtUnquote strips the outer quotes from a charmbracelet/log quoted value and
// unescapes only \" → ". charmbracelet does not double-escape backslashes (it writes
// \n as 0x5c 0x6e, not \\n), so strconv.Unquote is wrong here: it would convert \n
// into a real newline (0x0a), corrupting JSON escape sequences in the inner value.
func logfmtUnquote(s string) (string, bool) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", false
	}
	return strings.ReplaceAll(s[1:len(s)-1], `\"`, `"`), true
}

// removeRanges removes the byte ranges from s and returns the result.
// Ranges must be non-overlapping and sorted by start position.
func removeRanges(s string, ranges [][2]int) string {
	if len(ranges) == 0 {
		return s
	}
	var b strings.Builder
	prev := 0
	for _, r := range ranges {
		b.WriteString(s[prev:r[0]])
		prev = r[1]
	}
	b.WriteString(s[prev:])
	return b.String()
}
