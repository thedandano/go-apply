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
