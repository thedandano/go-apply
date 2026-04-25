package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Test 1: No quoted fields — a plain logfmt line with no key="value" pairs
// Output must be the input line unchanged (plus newline).
func TestRenderLine_NoQuotedFields(t *testing.T) {
	line := "2026-04-25 10:30:58 INFO starting tool=submit session_id=abc123 status=ok"
	var buf bytes.Buffer
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := line + "\n"
	if got := buf.String(); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// Test 2: Single JSON object field — result="{\"score\":75}"
// Header line must not contain result=..., JSON block appears below.
func TestRenderLine_SingleJSONObjectField(t *testing.T) {
	line := `level=info msg="tool done" result="{\"score\":75}"`
	var buf bytes.Buffer
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()

	if strings.Contains(got, `result="`) {
		t.Errorf("header should not contain result=..., got:\n%s", got)
	}
	if !strings.Contains(got, "  result:") {
		t.Errorf("expected '  result:' label in output, got:\n%s", got)
	}
	if !strings.Contains(got, `"score": 75`) {
		t.Errorf("expected pretty-printed JSON with \"score\": 75, got:\n%s", got)
	}
}

// Test 3: JSON array field — items="[1,2,3]"
// Header must not contain items=..., JSON array block appears below.
func TestRenderLine_JSONArrayField(t *testing.T) {
	line := `level=info msg="list" items="[1,2,3]"`
	var buf bytes.Buffer
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()

	if strings.Contains(got, `items="`) {
		t.Errorf("header should not contain items=..., got:\n%s", got)
	}
	if !strings.Contains(got, "  items:") {
		t.Errorf("expected '  items:' label in output, got:\n%s", got)
	}
	if !strings.Contains(got, "1,") || !strings.Contains(got, "2,") || !strings.Contains(got, "3") {
		t.Errorf("expected pretty-printed array values in output, got:\n%s", got)
	}
}

// Test 4: Multiple JSON fields — both moved below the header in original order.
func TestRenderLine_MultipleJSONFields(t *testing.T) {
	line := `level=info msg="done" result="{\"score\":80}" details="{\"tier\":\"t2\"}"`
	var buf bytes.Buffer
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()

	if strings.Contains(got, `result="`) {
		t.Errorf("header should not contain result=..., got:\n%s", got)
	}
	if strings.Contains(got, `details="`) {
		t.Errorf("header should not contain details=..., got:\n%s", got)
	}
	if !strings.Contains(got, "  result:") {
		t.Errorf("expected '  result:' block, got:\n%s", got)
	}
	if !strings.Contains(got, "  details:") {
		t.Errorf("expected '  details:' block, got:\n%s", got)
	}
	resultIdx := strings.Index(got, "  result:")
	detailsIdx := strings.Index(got, "  details:")
	if resultIdx > detailsIdx {
		t.Errorf("expected result block before details block (original order), got:\n%s", got)
	}
}

// Test 5: Quoted non-JSON string stays on header — status="ok" is not JSON object/array.
func TestRenderLine_QuotedNonJSONStringStaysOnHeader(t *testing.T) {
	line := `level=info msg="ping" status="ok"`
	var buf bytes.Buffer
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, `status="ok"`) {
		t.Errorf("expected status=\"ok\" on header line, got:\n%s", got)
	}
}

// Test 6: Malformed JSON stays on header, no panic — data="{broken" is invalid JSON.
func TestRenderLine_MalformedJSONStaysOnHeader(t *testing.T) {
	line := `level=info msg="bad" data="{broken"`
	var buf bytes.Buffer
	// Must not panic.
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()

	// The malformed field text should remain somewhere on the header.
	if !strings.Contains(got, `data="{broken"`) {
		t.Errorf("expected malformed field to stay on header line, got:\n%s", got)
	}
	if strings.Contains(got, "  data:") {
		t.Errorf("expected no 'data:' block for malformed JSON, got:\n%s", got)
	}
}

// Test 7: Empty line — renderLine("") produces empty output (no panic).
func TestRenderLine_EmptyLine(t *testing.T) {
	var buf bytes.Buffer
	if err := renderLine(&buf, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	// Empty line → write empty line + newline OR just nothing; either is acceptable
	// as long as it doesn't panic and output is just whitespace.
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty output for empty line, got: %q", got)
	}
}

// Test 8: Real-world logfmt line with embedded JSON.
// Header must contain non-JSON fields but NOT result=...; result block must appear below.
func TestRenderLine_RealWorldLogfmtLine(t *testing.T) {
	line := `2026-04-25 10:30:58 DEBU mcp tool result tool=submit_tailor_t2 status=ok result_bytes=1697 result="{\"previous_score\":75.875,\"new_score\":80.0}"`
	var buf bytes.Buffer
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()

	// Non-JSON fields must appear on header.
	headerLine := strings.SplitN(got, "\n", 2)[0]
	if !strings.Contains(headerLine, "tool=submit_tailor_t2") {
		t.Errorf("expected tool=submit_tailor_t2 on header, got: %q", headerLine)
	}
	if !strings.Contains(headerLine, "status=ok") {
		t.Errorf("expected status=ok on header, got: %q", headerLine)
	}
	if !strings.Contains(headerLine, "result_bytes=1697") {
		t.Errorf("expected result_bytes=1697 on header, got: %q", headerLine)
	}
	// JSON field must NOT be on header.
	if strings.Contains(headerLine, `result="`) {
		t.Errorf("result=... should not appear on header line, got: %q", headerLine)
	}
	// JSON block must appear below.
	if !strings.Contains(got, "  result:") {
		t.Errorf("expected '  result:' block below header, got:\n%s", got)
	}
	if !strings.Contains(got, "previous_score") {
		t.Errorf("expected pretty-printed JSON with previous_score field, got:\n%s", got)
	}
}

// Test 9: JSON primitive stays on header — result="42" is a valid JSON number, not object/array.
func TestRenderLine_JSONPrimitiveStaysOnHeader(t *testing.T) {
	line := `level=info msg="scored" result="42"`
	var buf bytes.Buffer
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, `result="42"`) {
		t.Errorf("expected result=\"42\" on header line (primitive JSON stays), got:\n%s", got)
	}
	if strings.Contains(got, "  result:") {
		t.Errorf("expected no result block for primitive JSON, got:\n%s", got)
	}
}

// Test 10: WARN-level line with JSON field renders identically to INFO-level line.
func TestRenderLine_WARNLevelSameFormattingAsINFO(t *testing.T) {
	infoLine := `level=info msg="done" result="{\"score\":90}"`
	warnLine := `level=warn msg="done" result="{\"score\":90}"`

	var infoBuf, warnBuf bytes.Buffer
	if err := renderLine(&infoBuf, infoLine); err != nil {
		t.Fatalf("info renderLine error: %v", err)
	}
	if err := renderLine(&warnBuf, warnLine); err != nil {
		t.Fatalf("warn renderLine error: %v", err)
	}

	infoGot := infoBuf.String()
	warnGot := warnBuf.String()

	// Both must have result block with identical formatting.
	if !strings.Contains(infoGot, "  result:") {
		t.Errorf("INFO output missing result block:\n%s", infoGot)
	}
	if !strings.Contains(warnGot, "  result:") {
		t.Errorf("WARN output missing result block:\n%s", warnGot)
	}

	// Strip the level= prefix to compare the JSON block formatting.
	infoIdx := strings.Index(infoGot, "  result:")
	warnIdx := strings.Index(warnGot, "  result:")
	if infoIdx < 0 || warnIdx < 0 {
		t.Fatal("result block not found in output; cannot compare formatting")
	}
	infoBlock := infoGot[infoIdx:]
	warnBlock := warnGot[warnIdx:]
	if infoBlock != warnBlock {
		t.Errorf("JSON block formatting differs between INFO and WARN:\nINFO: %q\nWARN: %q", infoBlock, warnBlock)
	}
}

// B1 regression: internal whitespace in non-JSON fields must be preserved verbatim.
// collapseSpaces was incorrectly rewriting "hello   world" to "hello world".
func TestRenderLine_PreservesInternalSpacesInNonJSONFields(t *testing.T) {
	line := `level=info msg="found   3  matches" status=ok result="{\"score\":75}"`
	var buf bytes.Buffer
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	const wantFragment = `msg="found   3  matches"`
	if !strings.Contains(got, wantFragment) {
		t.Errorf("internal whitespace was corrupted in header\nwant fragment: %q\ngot: %q", wantFragment, got)
	}
}

// B2 regression: hyphenated field keys must be matched by the regex and JSON
// fields with hyphenated names must be pretty-printed.
func TestRenderLine_HyphenatedFieldKey(t *testing.T) {
	line := `2026-04-25 10:00:00 INFO handler request-id=abc123 x-trace="{\"span\":\"abc\"}"`
	var buf bytes.Buffer
	if err := renderLine(&buf, line); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "  x-trace:") {
		t.Errorf("hyphenated JSON field not pretty-printed\ngot: %q", got)
	}
	if strings.Contains(got, `x-trace=`) {
		t.Errorf("hyphenated JSON field still on header line\ngot: %q", got)
	}
	if !strings.Contains(got, "request-id=abc123") {
		t.Errorf("non-JSON hyphenated field was removed from header\ngot: %q", got)
	}
}
