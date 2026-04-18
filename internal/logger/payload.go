package logger

import (
	"fmt"
	"log/slog"
	"regexp"
	"sync/atomic"
	"unicode/utf8"

	"github.com/thedandano/go-apply/internal/redact"
)

const payloadLimit = 2048

// globalRedactor is the process-level PII redactor. Nil means disabled.
var globalRedactor atomic.Pointer[redact.Redactor]

// SetRedactor installs r as the process-level PII redactor.
// After this call, all PayloadAttr string values are redacted before logging.
// Pass nil to disable redaction.
func SetRedactor(r *redact.Redactor) { globalRedactor.Store(r) }

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
// When a global PII redactor is installed via SetRedactor, it runs before the
// secret-pattern redaction.
func PayloadAttr(key, value string, verbose bool) slog.Attr {
	if r := globalRedactor.Load(); r != nil {
		value = r.Redact(value)
	}
	if verbose {
		return slog.String(key, Redact(value))
	}
	return slog.String(key, Truncate(Redact(value), payloadLimit))
}
