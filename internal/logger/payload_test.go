package logger

import (
	"context"
	"log/slog"
	"strings"
	"testing"
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
}

func TestDecision(t *testing.T) {
	t.Parallel()

	t.Run("logs debug record with expected fields", func(t *testing.T) {
		t.Parallel()
		records := make([]slog.Record, 0)
		handler := &captureHandler{records: &records, level: slog.LevelDebug}
		log := slog.New(handler)
		ctx := context.Background()

		Decision(ctx, log, "fetcher.source", "cache", "cache hit")

		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		r := records[0]
		if r.Level != slog.LevelDebug {
			t.Errorf("expected DEBUG level, got %v", r.Level)
		}
		if r.Message != "decision" {
			t.Errorf("expected message 'decision', got %q", r.Message)
		}
		fields := recordAttrs(r)
		if fields["name"] != "fetcher.source" {
			t.Errorf("expected name='fetcher.source', got %q", fields["name"])
		}
		if fields["chosen"] != "cache" {
			t.Errorf("expected chosen='cache', got %q", fields["chosen"])
		}
		if fields["reason"] != "cache hit" {
			t.Errorf("expected reason='cache hit', got %q", fields["reason"])
		}
	})

	t.Run("extra attrs are included", func(t *testing.T) {
		t.Parallel()
		records := make([]slog.Record, 0)
		handler := &captureHandler{records: &records, level: slog.LevelDebug}
		log := slog.New(handler)
		ctx := context.Background()

		Decision(ctx, log, "tailor.tier", "t1", "score below threshold",
			slog.Int("score", 42),
		)

		if len(records) != 1 {
			t.Fatalf("expected 1 record, got %d", len(records))
		}
		fields := recordAttrs(records[0])
		if fields["score"] != "42" {
			t.Errorf("expected score=42, got %q", fields["score"])
		}
	})

	t.Run("smoke test does not panic", func(t *testing.T) {
		t.Parallel()
		log := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
		Decision(context.Background(), log, "x", "y", "z")
	})
}

// captureHandler captures slog.Records for inspection in tests.
type captureHandler struct {
	records *[]slog.Record
	level   slog.Level
}

func (h *captureHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires slog.Record by value
	*h.records = append(*h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

// recordAttrs converts a slog.Record's attrs to a string map for easy assertions.
func recordAttrs(r slog.Record) map[string]string { //nolint:gocritic // hugeParam: slog.Handler interface requires slog.Record by value
	m := make(map[string]string)
	r.Attrs(func(a slog.Attr) bool {
		m[a.Key] = a.Value.String()
		return true
	})
	return m
}
