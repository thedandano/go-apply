package redact

import (
	"reflect"
	"regexp"
	"strings"
)

// Profile holds PII literal strings derived from the user's profile.
// Each field is optional (empty string = no replacement for that field).
type Profile struct {
	Name        string
	Email       string
	Phone       string
	Location    string
	LinkedInURL string
}

// Redactor masks PII in strings. It applies literal replacements first
// (case-insensitive, word-boundary aware for Name to avoid over-matching),
// then regex patterns for email and phone formats not covered by literal matching.
type Redactor struct {
	profile  Profile
	patterns []patternReplacement
}

type patternReplacement struct {
	re    *regexp.Regexp
	token string
}

// builtinPatterns covers common PII formats.
// E.164 is listed before NANP to prevent NANP from greedily consuming the
// digit portion of a +1-prefixed number and leaving stray characters.
var builtinPatterns = []patternReplacement{
	// RFC-5322-style email
	{regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`), "«EMAIL»"},
	// E.164: +15555551234 — must come before NANP
	{regexp.MustCompile(`\+1?\d{10,14}`), "«PHONE»"},
	// NANP explicit parens: (555)555-1234 / (555) 555-1234 / (555)5551234
	// Parens imply phone context so no separator after ) is required.
	{regexp.MustCompile(`\(\d{3}\)\s?\d{3}[\s.\-]?\d{4}`), "«PHONE»"},
	// NANP with separator (no parens): 555-555-1234 / 555.555.1234 / 555 555-1234
	// Separator after area code is required — digit runs in floats never have one.
	{regexp.MustCompile(`\d{3}[\s.\-]\d{3}[\s.\-]?\d{4}`), "«PHONE»"},
	// NANP bare 10-digit: 5555551234
	// Leading boundary excludes digits AND dots so decimal fractions (52.6666666666)
	// are not matched. Trailing boundary excludes digits only — dots are allowed
	// there so sentence-terminal phones (5555551234.) are still redacted.
	// Go regexp has no lookbehind; surrounding chars are captured and restored.
	{regexp.MustCompile(`([^\d.]|^)\d{10}([^\d]|$)`), "${1}«PHONE»${2}"},
}

// New constructs a Redactor from a profile. Fields that are empty strings
// are skipped — no replacement is made for absent profile data.
func New(p *Profile) *Redactor {
	return &Redactor{
		profile:  *p,
		patterns: builtinPatterns,
	}
}

// Redact masks PII in s. Returns the redacted string.
func (r *Redactor) Redact(s string) string {
	// Literal replacements first.
	if r.profile.Email != "" {
		s = replaceCI(s, r.profile.Email, "«EMAIL»")
	}
	if r.profile.Phone != "" {
		s = replaceCI(s, r.profile.Phone, "«PHONE»")
	}
	if r.profile.LinkedInURL != "" {
		s = replaceCI(s, r.profile.LinkedInURL, "«URL»")
	}
	if r.profile.Location != "" {
		s = replaceCI(s, r.profile.Location, "«LOCATION»")
	}
	if r.profile.Name != "" {
		// Use word-boundary replacement to avoid matching name substrings within other words.
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(r.profile.Name) + `\b`)
		s = re.ReplaceAllString(s, "«NAME»")
	}
	// Regex patterns for formats not covered by literal matching.
	for _, p := range r.patterns {
		s = p.re.ReplaceAllString(s, p.token)
	}
	return s
}

// RedactAny recursively walks v (struct, map, slice, or scalar) and redacts
// all string values. Non-string, non-container values are returned unchanged.
// Returns a new value; does not modify the original.
func (r *Redactor) RedactAny(v any) any {
	return r.redactValue(reflect.ValueOf(v)).Interface()
}

func (r *Redactor) redactValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.String:
		return reflect.ValueOf(r.Redact(v.String()))
	case reflect.Ptr:
		if v.IsNil() {
			return v
		}
		result := reflect.New(v.Type().Elem())
		result.Elem().Set(r.redactValue(v.Elem()))
		return result
	case reflect.Struct:
		result := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				result.Field(i).Set(r.redactValue(v.Field(i)))
			}
			// unexported fields: leave zero — cannot be accessed cross-package
		}
		return result
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		result := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			result.Index(i).Set(r.redactValue(v.Index(i)))
		}
		return result
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		result := reflect.MakeMap(v.Type())
		for _, key := range v.MapKeys() {
			result.SetMapIndex(r.redactValue(key), r.redactValue(v.MapIndex(key)))
		}
		return result
	default:
		return v
	}
}

// replaceCI replaces all case-insensitive occurrences of old with replacement in s.
func replaceCI(s, old, replacement string) string {
	if old == "" {
		return s
	}
	lowerS := strings.ToLower(s)
	lowerOld := strings.ToLower(old)
	var result strings.Builder
	result.Grow(len(s))
	for {
		idx := strings.Index(lowerS, lowerOld)
		if idx == -1 {
			result.WriteString(s)
			break
		}
		result.WriteString(s[:idx])
		result.WriteString(replacement)
		s = s[idx+len(old):]
		lowerS = lowerS[idx+len(old):]
	}
	return result.String()
}
