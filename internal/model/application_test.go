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
			TierApplied:   TierKeyword,
			TailoredText:  "body",
			AddedKeywords: []string{"go", "kubernetes"},
			NewScore:      ScoreResult{ResumeLabel: "my-resume"},
			Tier1Score:    &ScoreResult{ResumeLabel: "my-resume"},
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
	requiredKeys := []string{`"tier_applied"`, `"added_keywords"`, `"new_score"`, `"tier1_score"`}
	for _, key := range requiredKeys {
		if !strings.Contains(string(out), key) {
			t.Errorf("serialized ApplicationRecord missing key %s; json = %s", key, out)
		}
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

func TestApplicationRecord_MarshalJSON_ChangelogAndRawChangelogSurvive(t *testing.T) {
	rec := ApplicationRecord{
		URL: "https://example.com/job/999",
		TailorResult: &TailorResult{
			ResumeLabel:  "cv",
			TailoredText: "must be redacted",
			RawChangelog: "raw-log-data",
			Changelog: []ChangelogEntry{
				{Kind: ChangelogSkillAdd, Tier: ChangelogTier1, Keyword: "kubernetes"},
			},
		},
	}

	out, err := json.Marshal(&rec)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	s := string(out)

	if strings.Contains(s, `"tailored_text"`) {
		t.Errorf("tailored_text must be redacted; json = %s", s)
	}
	if !strings.Contains(s, `"raw_changelog"`) {
		t.Errorf("raw_changelog must survive MarshalJSON; json = %s", s)
	}
	if !strings.Contains(s, `"changelog"`) {
		t.Errorf("changelog must survive MarshalJSON; json = %s", s)
	}
	if !strings.Contains(s, `"skill_add"`) {
		t.Errorf("changelog entry kind must be in JSON; json = %s", s)
	}
	if rec.TailorResult.TailoredText != "must be redacted" {
		t.Errorf("in-memory TailoredText mutated; got %q", rec.TailorResult.TailoredText)
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
			TierApplied:  TierBullet,
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
