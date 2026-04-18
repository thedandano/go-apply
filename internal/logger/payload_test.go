package logger

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncate(t *testing.T) {
	t.Parallel()

	t.Run("within limit returns unchanged", func(t *testing.T) {
		t.Parallel()
		s := "hello world"
		got := Truncate(s, 100)
		if got != s {
			t.Errorf("expected %q, got %q", s, got)
		}
	})

	t.Run("exactly at limit returns unchanged", func(t *testing.T) {
		t.Parallel()
		s := "hello"
		got := Truncate(s, 5)
		if got != s {
			t.Errorf("expected %q, got %q", s, got)
		}
	})

	t.Run("over limit returns head+marker+tail", func(t *testing.T) {
		t.Parallel()
		s := strings.Repeat("a", 100) + strings.Repeat("b", 100)
		got := Truncate(s, 10)
		// head = s[:5] = "aaaaa", tail = s[195:] = "bbbbb", omitted = 200-10 = 190
		if !strings.HasPrefix(got, "aaaaa") {
			t.Errorf("expected head 'aaaaa', got prefix %q", got[:min(5, len(got))])
		}
		if !strings.HasSuffix(got, "bbbbb") {
			t.Errorf("expected tail 'bbbbb', got suffix %q", got[max(0, len(got)-5):])
		}
		if !strings.Contains(got, "[190 bytes omitted]") {
			t.Errorf("expected omitted marker, got %q", got)
		}
	})

	t.Run("limit 0 returns empty", func(t *testing.T) {
		t.Parallel()
		got := Truncate("anything", 0)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("negative limit returns empty", func(t *testing.T) {
		t.Parallel()
		got := Truncate("anything", -5)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("empty string within limit", func(t *testing.T) {
		t.Parallel()
		got := Truncate("", 10)
		if got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("multi-byte UTF-8 truncation produces valid UTF-8", func(t *testing.T) {
		t.Parallel()
		// "é" is a 2-byte rune; 2000 of them = 4000 bytes, well over limit=10
		got := Truncate(strings.Repeat("é", 2000), 10)
		if !utf8.ValidString(got) {
			t.Errorf("expected valid UTF-8 after truncation, got invalid string: %q", got)
		}
	})
}

func TestRedact(t *testing.T) {
	t.Parallel()

	t.Run("API key pattern colon separator", func(t *testing.T) {
		t.Parallel()
		input := "api_key: secret123"
		got := Redact(input)
		if strings.Contains(got, "secret123") {
			t.Errorf("expected secret to be redacted, got %q", got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("expected [REDACTED] marker, got %q", got)
		}
	})

	t.Run("API key pattern equals separator", func(t *testing.T) {
		t.Parallel()
		input := "apikey=mysecretvalue"
		got := Redact(input)
		if strings.Contains(got, "mysecretvalue") {
			t.Errorf("expected secret to be redacted, got %q", got)
		}
	})

	t.Run("Bearer token", func(t *testing.T) {
		t.Parallel()
		input := "Authorization: Bearer eyJhbGciOiJSUzI1NiJ9"
		got := Redact(input)
		if strings.Contains(got, "eyJhbGciOiJSUzI1NiJ9") {
			t.Errorf("expected Bearer token to be redacted, got %q", got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("expected [REDACTED] marker, got %q", got)
		}
	})

	t.Run("sk- prefixed key", func(t *testing.T) {
		t.Parallel()
		input := "key=sk-abcdefghij1234567890"
		got := Redact(input)
		if strings.Contains(got, "sk-abcdefghij1234567890") {
			t.Errorf("expected sk- key to be redacted, got %q", got)
		}
	})

	t.Run("email address", func(t *testing.T) {
		t.Parallel()
		input := "user email is dan@example.com for the request"
		got := Redact(input)
		if strings.Contains(got, "dan@example.com") {
			t.Errorf("expected email to be redacted, got %q", got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("expected [REDACTED] marker, got %q", got)
		}
	})

	t.Run("multiple patterns in one string", func(t *testing.T) {
		t.Parallel()
		input := "Bearer tokenABC api_key: supersecret user@test.org sk-longkeyvalue12345"
		got := Redact(input)
		if strings.Contains(got, "tokenABC") {
			t.Errorf("Bearer token not redacted: %q", got)
		}
		if strings.Contains(got, "supersecret") {
			t.Errorf("api_key not redacted: %q", got)
		}
		if strings.Contains(got, "user@test.org") {
			t.Errorf("email not redacted: %q", got)
		}
		if strings.Contains(got, "sk-longkeyvalue12345") {
			t.Errorf("sk- key not redacted: %q", got)
		}
	})

	t.Run("clean string returns unchanged", func(t *testing.T) {
		t.Parallel()
		input := "this is a safe log message with no secrets"
		got := Redact(input)
		if got != input {
			t.Errorf("expected %q unchanged, got %q", input, got)
		}
	})
}

func TestPayloadAttr(t *testing.T) {
	t.Parallel()

	longValue := strings.Repeat("x", payloadLimit+100)

	t.Run("verbose=false applies truncation and redaction", func(t *testing.T) {
		t.Parallel()
		value := longValue + " api_key: secret99"
		attr := PayloadAttr("body", value, false)
		result := attr.Value.String()
		// Truncation should have occurred
		if len(result) >= len(value) {
			t.Errorf("expected truncation for long value")
		}
		// Redaction should have occurred
		if strings.Contains(result, "secret99") {
			t.Errorf("expected secret to be redacted, got %q", result)
		}
		// either the api_key was in the omitted range OR redacted — both are acceptable outcomes
	})

	t.Run("verbose=false truncates short value but still redacts", func(t *testing.T) {
		t.Parallel()
		value := "api_key: topsecret"
		attr := PayloadAttr("body", value, false)
		result := attr.Value.String()
		if strings.Contains(result, "topsecret") {
			t.Errorf("expected secret to be redacted, got %q", result)
		}
	})

	t.Run("verbose=true skips truncation even for long string", func(t *testing.T) {
		t.Parallel()
		attr := PayloadAttr("body", longValue, true)
		result := attr.Value.String()
		// No truncation marker expected
		if strings.Contains(result, "bytes omitted") {
			t.Errorf("verbose=true should not truncate, but got truncation marker: %q", result[:100])
		}
		// Length should be preserved (no truncation)
		if len(result) < payloadLimit {
			t.Errorf("verbose=true should preserve full length, got len=%d", len(result))
		}
	})

	t.Run("verbose=true still redacts secrets", func(t *testing.T) {
		t.Parallel()
		value := "Bearer mytoken123"
		attr := PayloadAttr("body", value, true)
		result := attr.Value.String()
		if strings.Contains(result, "mytoken123") {
			t.Errorf("verbose=true should still redact secrets, got %q", result)
		}
	})

	t.Run("attr key is preserved", func(t *testing.T) {
		t.Parallel()
		attr := PayloadAttr("request_body", "hello", false)
		if attr.Key != "request_body" {
			t.Errorf("expected key 'request_body', got %q", attr.Key)
		}
	})

	t.Run("secret straddling truncation boundary is redacted", func(t *testing.T) {
		t.Parallel()
		// Build a string long enough to trigger truncation, with sk- key placed
		// so it straddles the natural truncation cut point (payloadLimit/2).
		half := payloadLimit / 2
		// Place the secret so its start is just before the head cut and its end is just after.
		prefix := strings.Repeat("x", half-5)
		secret := "sk-abcdefghij1234567890"
		suffix := strings.Repeat("y", payloadLimit) // ensures total len > payloadLimit
		s := prefix + secret + suffix
		attr := PayloadAttr("k", s, false)
		result := attr.Value.String()
		if strings.Contains(result, "sk-") {
			t.Errorf("expected sk- secret to be redacted even when straddling truncation boundary, got: %q", result[:min(200, len(result))])
		}
	})
}
