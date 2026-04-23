package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationRecord_MarshalJSON_RedactsTailoredText(t *testing.T) {
	rec := ApplicationRecord{
		URL:     "https://example.com/job/123",
		RawText: "raw job description",
		TailorResult: &TailorResult{
			ResumeLabel:   "my-resume",
			TailoredText:  "body",
			AddedKeywords: []string{"go", "kubernetes"},
			NewScore:      ScoreResult{ResumeLabel: "my-resume"},
			Changelog: []ChangelogEntry{
				{Action: "added", Target: "skill", Keyword: "go", Reason: "required"},
				{Action: "rewrote", Target: "bullet", Keyword: "kubernetes"},
			},
		},
	}

	out, err := json.Marshal(&rec)
	if err != nil {
		t.Fatalf("json.Marshal(ApplicationRecord) error = %v", err)
	}

	// "tailored_text" must NOT appear in the serialized bytes.
	if strings.Contains(string(out), `"tailored_text"`) {
		t.Errorf("serialized ApplicationRecord contains \"tailored_text\"; it must be redacted; json = %s", out)
	}

	// Other TailorResult fields must still be present.
	requiredKeys := []string{`"added_keywords"`, `"new_score"`, `"changelog"`}
	for _, key := range requiredKeys {
		if !strings.Contains(string(out), key) {
			t.Errorf("serialized ApplicationRecord missing key %s; json = %s", key, out)
		}
	}

	// Changelog entries must round-trip losslessly.
	var decoded ApplicationRecord
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("unmarshal redacted record: %v", err)
	}
	if decoded.TailorResult == nil {
		t.Fatal("TailorResult nil after unmarshal")
	}
	if len(decoded.TailorResult.Changelog) != 2 {
		t.Fatalf("Changelog length = %d, want 2", len(decoded.TailorResult.Changelog))
	}
	if decoded.TailorResult.Changelog[0].Action != "added" || decoded.TailorResult.Changelog[0].Keyword != "go" {
		t.Errorf("Changelog[0] = %+v, unexpected", decoded.TailorResult.Changelog[0])
	}

	// The in-memory receiver must be unchanged after marshal.
	if rec.TailorResult.TailoredText != "body" {
		t.Errorf("in-memory TailoredText was mutated; got %q, want %q", rec.TailorResult.TailoredText, "body")
	}
}

func TestApplicationRecord_MarshalJSON_NilTailorResult(t *testing.T) {
	rec := ApplicationRecord{
		URL:          "https://example.com/job/456",
		TailorResult: nil,
	}

	out, err := json.Marshal(&rec)
	if err != nil {
		t.Fatalf("json.Marshal(ApplicationRecord) with nil TailorResult error = %v", err)
	}

	// tailor_result should be omitted when nil.
	if strings.Contains(string(out), `"tailor_result"`) {
		t.Errorf("nil TailorResult should be omitted; json = %s", out)
	}
}

func TestApplicationRecord_MarshalJSON_Round_Trip_WithChangelog(t *testing.T) {
	const sentinelText = "FULL TAILORED BODY THAT MUST BE REDACTED"

	rec := ApplicationRecord{
		URL: "https://example.com/job/123",
		JD:  JDData{Title: "Software Engineer", Company: "Acme Corp"},
		Score: &ScoreResult{
			ResumeLabel: "my-resume",
			Breakdown:   ScoreBreakdown{KeywordMatch: 20, ExperienceFit: 18},
		},
		ResumeLabel: "my-resume",
		TailorResult: &TailorResult{
			ResumeLabel:  "my-resume",
			TailoredText: sentinelText,
			Changelog: []ChangelogEntry{
				{Action: "added", Target: "skill", Keyword: "kubernetes", Reason: "required by JD"},
				{Action: "rewrote", Target: "bullet", Keyword: "aws", Reason: "broader scope"},
				{Action: "skipped", Target: "skill", Keyword: "rust", Reason: "no accomplishments basis"},
			},
			NewScore: ScoreResult{ResumeLabel: "my-resume", Breakdown: ScoreBreakdown{KeywordMatch: 25}},
		},
		CoverLetter: "agent-supplied cover letter",
	}

	data, err := json.Marshal(&rec)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	jsonStr := string(data)

	// 1. Sentinel body and tailored_text key must not appear in serialized bytes.
	if strings.Contains(jsonStr, sentinelText) {
		t.Errorf("serialized bytes contain sentinel text; redaction failed; json = %s", jsonStr)
	}
	if strings.Contains(jsonStr, `"tailored_text"`) {
		t.Errorf("serialized bytes contain \"tailored_text\" key; redaction failed; json = %s", jsonStr)
	}

	// 2. All three changelog keywords, actions, and reasons must be present.
	for _, want := range []string{"kubernetes", "required by JD", "aws", "broader scope", "rust", "no accomplishments basis"} {
		if !strings.Contains(jsonStr, want) {
			t.Errorf("serialized bytes missing expected changelog value %q; json = %s", want, jsonStr)
		}
	}

	// 3. Unmarshal back and verify changelog round-trips unchanged.
	var rec2 ApplicationRecord
	if err := json.Unmarshal(data, &rec2); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if rec2.TailorResult == nil {
		t.Fatal("TailorResult nil after unmarshal")
	}
	if len(rec2.TailorResult.Changelog) != 3 {
		t.Fatalf("Changelog length = %d, want 3", len(rec2.TailorResult.Changelog))
	}
	want := []ChangelogEntry{
		{Action: "added", Target: "skill", Keyword: "kubernetes", Reason: "required by JD"},
		{Action: "rewrote", Target: "bullet", Keyword: "aws", Reason: "broader scope"},
		{Action: "skipped", Target: "skill", Keyword: "rust", Reason: "no accomplishments basis"},
	}
	for i, w := range want {
		got := rec2.TailorResult.Changelog[i]
		if got != w {
			t.Errorf("Changelog[%d] = %+v, want %+v", i, got, w)
		}
	}

	// 4. In-memory receiver must not be mutated by marshal.
	if rec.TailorResult.TailoredText != sentinelText {
		t.Errorf("in-memory TailoredText mutated; got %q, want %q", rec.TailorResult.TailoredText, sentinelText)
	}
}

func TestApplicationRecord_MarshalJSON_PreservesOtherFields(t *testing.T) {
	score := &ScoreResult{ResumeLabel: "cv"}
	rec := ApplicationRecord{
		URL:         "https://example.com/job/789",
		CoverLetter: "Dear Hiring Manager,",
		Score:       score,
		TailorResult: &TailorResult{
			ResumeLabel:  "cv",
			TailoredText: "full text here",
		},
		Applied: "2026-04-22",
		Outcome: OutcomePending,
	}

	out, err := json.Marshal(&rec)
	if err != nil {
		t.Fatalf("json.Marshal(ApplicationRecord) error = %v", err)
	}

	checkPresent := []string{`"url"`, `"cover_letter"`, `"score"`, `"tailor_result"`, `"applied"`, `"outcome"`}
	for _, key := range checkPresent {
		if !strings.Contains(string(out), key) {
			t.Errorf("serialized ApplicationRecord missing key %s; json = %s", key, out)
		}
	}

	if strings.Contains(string(out), `"tailored_text"`) {
		t.Errorf("serialized ApplicationRecord must not contain \"tailored_text\"; json = %s", out)
	}
}
