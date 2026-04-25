package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// quotedFieldRe matches logfmt key="value" pairs where the value is a quoted string.
var quotedFieldRe = regexp.MustCompile(`(\w+)=("[^"\\]*(?:\\.[^"\\]*)*")`)

// jsonField holds a key and its parsed JSON value extracted from a log line.
type jsonField struct {
	key       string
	fullMatch string // the original key="value" text to remove from header
	parsed    json.RawMessage
}

// renderLine writes a pretty-printed representation of a logfmt log line to w.
// Fields whose quoted string values are valid JSON objects or arrays are removed
// from the header line and printed below it, indented, under a "key:" label.
// Lines containing no JSON-valued fields are written unchanged.
// Errors from w are returned.
func renderLine(w io.Writer, line string) error {
	matches := quotedFieldRe.FindAllStringSubmatchIndex(line, -1)

	var fields []jsonField
	removedRanges := make([][2]int, 0)

	for _, m := range matches {
		// m[0]:m[1] = full match, m[2]:m[3] = key, m[4]:m[5] = quoted value
		fullMatch := line[m[0]:m[1]]
		key := line[m[2]:m[3]]
		rawValue := line[m[4]:m[5]]

		unquoted, err := strconv.Unquote(rawValue)
		if err != nil {
			continue
		}

		var raw json.RawMessage
		if err := json.Unmarshal([]byte(unquoted), &raw); err != nil {
			continue
		}

		if len(unquoted) == 0 {
			continue
		}

		firstByte := unquoted[0]
		if firstByte != '{' && firstByte != '[' {
			// JSON primitive — stays on header
			continue
		}

		fields = append(fields, jsonField{
			key:       key,
			fullMatch: fullMatch,
			parsed:    raw,
		})
		removedRanges = append(removedRanges, [2]int{m[0], m[1]})
	}

	// Build header by removing JSON field matches.
	header := removeRanges(line, removedRanges)
	// Collapse multiple spaces and trim trailing whitespace.
	header = collapseSpaces(header)

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

// collapseSpaces replaces runs of spaces with a single space and trims trailing whitespace.
func collapseSpaces(s string) string {
	// Collapse multiple consecutive spaces into one.
	result := strings.Join(strings.Fields(s), " ")
	return result
}
