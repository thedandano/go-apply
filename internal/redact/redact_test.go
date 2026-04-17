package redact_test

import (
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/redact"
)

func TestRedact_NameWordBoundary(t *testing.T) {
	r := redact.New(&redact.Profile{Name: "Dan"})

	// Should NOT redact substring inside another word.
	got := r.Redact("I love to Dance and be Dandy")
	if got != "I love to Dance and be Dandy" {
		t.Errorf("word-boundary violation: got %q", got)
	}

	// Should redact when the name stands alone.
	got = r.Redact("Dan Smith is awesome")
	if got != "«NAME» Smith is awesome" {
		t.Errorf("expected name redacted: got %q", got)
	}
}

func TestRedact_EmailLiteral(t *testing.T) {
	r := redact.New(&redact.Profile{Email: "dan@example.com"})
	got := r.Redact("contact me at Dan@Example.COM please")
	if got != "contact me at «EMAIL» please" {
		t.Errorf("email literal: got %q", got)
	}
}

func TestRedact_PhoneParenDash(t *testing.T) {
	r := redact.New(&redact.Profile{})
	got := r.Redact("call (555) 555-1234 now")
	if got != "call «PHONE» now" {
		t.Errorf("NANP paren-dash: got %q", got)
	}
}

func TestRedact_PhoneDashFormat(t *testing.T) {
	r := redact.New(&redact.Profile{})
	got := r.Redact("555-555-1234")
	if got != "«PHONE»" {
		t.Errorf("NANP dash: got %q", got)
	}
}

func TestRedact_PhoneE164(t *testing.T) {
	r := redact.New(&redact.Profile{})
	got := r.Redact("call +15555551234 for info")
	if got != "call «PHONE» for info" {
		t.Errorf("E.164: got %q", got)
	}
}

func TestRedact_LocationLiteral(t *testing.T) {
	r := redact.New(&redact.Profile{Location: "San Francisco, CA"})
	got := r.Redact("I live in san francisco, ca area")
	if got != "I live in «LOCATION» area" {
		t.Errorf("location literal: got %q", got)
	}
}

func TestRedact_NameCaseInsensitive(t *testing.T) {
	r := redact.New(&redact.Profile{Name: "Jane Smith"})
	got := r.Redact("JANE SMITH is the candidate")
	if got != "«NAME» is the candidate" {
		t.Errorf("case insensitive name: got %q", got)
	}
}

func TestRedactAny_Struct(t *testing.T) {
	type Profile struct {
		Name  string
		Score int
		Tags  []string
	}

	r := redact.New(&redact.Profile{Name: "Alice"})
	in := Profile{
		Name:  "Alice Smith",
		Score: 42,
		Tags:  []string{"Alice", "engineer"},
	}
	out := r.RedactAny(in).(Profile)

	if out.Name != "«NAME» Smith" {
		t.Errorf("struct.Name: got %q", out.Name)
	}
	if out.Score != 42 {
		t.Errorf("struct.Score mutated: got %d", out.Score)
	}
	if out.Tags[0] != "«NAME»" {
		t.Errorf("struct.Tags[0]: got %q", out.Tags[0])
	}
	if out.Tags[1] != "engineer" {
		t.Errorf("struct.Tags[1]: got %q", out.Tags[1])
	}
}

func TestRedactAny_StructWithUnexportedFields_NoPanic(t *testing.T) {
	type inner struct {
		Public  string
		private string //nolint:unused
	}
	r := redact.New(&redact.Profile{Name: "Alice"})
	v := inner{Public: "Alice works here", private: "secret"}
	got := r.RedactAny(v)
	result := got.(inner)
	if strings.Contains(result.Public, "Alice") {
		t.Errorf("expected Public field to be redacted, got: %s", result.Public)
	}
}

func TestRedact_EmptyProfileNoReplacement(t *testing.T) {
	r := redact.New(&redact.Profile{})
	// No literal PII — only regex patterns may fire, but not on this text.
	input := "Hello World, no PII here at all!"
	got := r.Redact(input)
	if got != input {
		t.Errorf("empty profile changed string: got %q", got)
	}
}
