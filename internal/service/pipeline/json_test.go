package pipeline

import (
	"testing"
)

func TestParseJSONFromResponse_RawJSON(t *testing.T) {
	resp := `{"title":"SWE","company":"Acme"}`
	var got map[string]string
	if err := parseJSONFromResponse(resp, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["title"] != "SWE" {
		t.Errorf("title = %q, want SWE", got["title"])
	}
	if got["company"] != "Acme" {
		t.Errorf("company = %q, want Acme", got["company"])
	}
}

func TestParseJSONFromResponse_MarkdownFenced(t *testing.T) {
	resp := "Here is the JSON:\n```json\n{\"title\":\"Engineer\",\"company\":\"Corp\"}\n```"
	var got map[string]string
	if err := parseJSONFromResponse(resp, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["title"] != "Engineer" {
		t.Errorf("title = %q, want Engineer", got["title"])
	}
}

func TestParseJSONFromResponse_FencedNoLanguage(t *testing.T) {
	resp := "Result:\n```\n{\"key\":\"value\"}\n```"
	var got map[string]string
	if err := parseJSONFromResponse(resp, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["key"] != "value" {
		t.Errorf("key = %q, want value", got["key"])
	}
}

func TestParseJSONFromResponse_EmbeddedInProse(t *testing.T) {
	resp := `Here is the extracted data: {"title":"Lead","company":"X"} — hope that helps.`
	var got map[string]string
	if err := parseJSONFromResponse(resp, &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["title"] != "Lead" {
		t.Errorf("title = %q, want Lead", got["title"])
	}
}

func TestParseJSONFromResponse_NoJSON_ReturnsError(t *testing.T) {
	resp := "Sorry, I cannot extract any JSON from this."
	var got map[string]string
	if err := parseJSONFromResponse(resp, &got); err == nil {
		t.Fatal("expected error for response with no JSON, got nil")
	}
}

func TestParseJSONFromResponse_MissingClosingBrace(_ *testing.T) {
	// Simulates a malformed LLM response with no closing brace but has an opening brace.
	resp := `{"title":"incomplete"`
	var got map[string]string
	// json.Unmarshal will fail — that's fine, we just want no panic.
	_ = parseJSONFromResponse(resp, &got)
}
