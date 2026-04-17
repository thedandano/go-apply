package logger

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"unicode/utf8"
)

const payloadLimit = 2048

// secretPatterns are compiled once at package init.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|apikey|api_secret)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)Bearer\s+\S+`),
	regexp.MustCompile(`sk-[A-Za-z0-9\-_]{10,}`),
	regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
}

// Truncate returns s with retained content of at most limit bytes (plus omission marker),
// with a marker showing how many bytes were omitted.
// If s fits within limit, it is returned unchanged.
// If limit <= 0, an empty string is returned.
func Truncate(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(s) <= limit {
		return s
	}
	half := limit / 2

	// snap head boundary back to a rune start
	headEnd := half
	for headEnd > 0 && !utf8.RuneStart(s[headEnd]) {
		headEnd--
	}
	head := s[:headEnd]

	// snap tail boundary forward to a rune start
	tailStart := len(s) - half
	for tailStart < len(s) && !utf8.RuneStart(s[tailStart]) {
		tailStart++
	}
	tail := s[tailStart:]

	omitted := len(s) - headEnd - (len(s) - tailStart)
	return head + fmt.Sprintf(" … [%d bytes omitted] … ", omitted) + tail
}

// Redact replaces known secret patterns in s with "[REDACTED]".
func Redact(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// PayloadAttr returns an slog.Attr for a payload value.
// If verbose is true, only Redact is applied (no truncation).
// If verbose is false, both Truncate (payloadLimit bytes) and Redact are applied.
func PayloadAttr(key, value string, verbose bool) slog.Attr {
	if verbose {
		return slog.String(key, Redact(value))
	}
	return slog.String(key, Truncate(Redact(value), payloadLimit))
}

// Decision logs a branch decision at DEBUG level.
// name:   the decision point (e.g. "fetcher.source", "tailor.tier")
// chosen: which branch was taken (e.g. "cache", "t1")
// reason: why (e.g. "cache hit", "score below threshold")
// attrs:  optional additional slog.Attr for context
func Decision(ctx context.Context, log *slog.Logger, name, chosen, reason string, attrs ...slog.Attr) {
	args := []any{
		slog.String("name", name),
		slog.String("chosen", chosen),
		slog.String("reason", reason),
	}
	for _, a := range attrs {
		args = append(args, a)
	}
	log.DebugContext(ctx, "decision", args...)
}
