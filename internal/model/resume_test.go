package model

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestTailorResult_MarshalJSON_TailoredText(t *testing.T) {
	tests := []struct {
		name         string
		tailoredText string
		wantKey      bool // whether "tailored_text" should appear in JSON
	}{
		{
			name:         "non-empty TailoredText is serialized",
			tailoredText: "hello",
			wantKey:      true,
		},
		{
			name:         "empty TailoredText is omitted",
			tailoredText: "",
			wantKey:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := TailorResult{
				ResumeLabel:  "test-resume",
				TierApplied:  TierKeyword,
				TailoredText: tt.tailoredText,
			}

			out, err := json.Marshal(r)
			if err != nil {
				t.Fatalf("json.Marshal(TailorResult) error = %v", err)
			}

			keyPresent := bytes.Contains(out, []byte(`"tailored_text"`))
			if keyPresent != tt.wantKey {
				t.Errorf("json output key presence = %v, want %v; json = %s", keyPresent, tt.wantKey, out)
			}

			if tt.wantKey {
				expected := []byte(`"tailored_text":"hello"`)
				if !bytes.Contains(out, expected) {
					t.Errorf("json output does not contain %s; got %s", expected, out)
				}
			}
		})
	}
}

func TestTailorResult_RoundTrip_TailoredText(t *testing.T) {
	original := TailorResult{
		ResumeLabel:  "round-trip-resume",
		TierApplied:  TierBullet,
		TailoredText: "full tailored body",
		Tier1Text:    "tier1 only body",
	}

	out, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}

	var decoded TailorResult
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}

	if decoded.TailoredText != original.TailoredText {
		t.Errorf("TailoredText after round-trip = %q, want %q", decoded.TailoredText, original.TailoredText)
	}
	if decoded.Tier1Text != original.Tier1Text {
		t.Errorf("Tier1Text after round-trip = %q, want %q", decoded.Tier1Text, original.Tier1Text)
	}
	if decoded.ResumeLabel != original.ResumeLabel {
		t.Errorf("ResumeLabel after round-trip = %q, want %q", decoded.ResumeLabel, original.ResumeLabel)
	}
}

// --- T010: Changelog JSON round-trip per kind ---

func roundTripChangelogEntry(t *testing.T, entry *ChangelogEntry) {
	t.Helper()
	out, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	var decoded ChangelogEntry
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if decoded != *entry {
		t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", decoded, *entry)
	}
}

func TestChangelogEntry_JSONRoundTrip_SkillAdd(t *testing.T) {
	roundTripChangelogEntry(t, &ChangelogEntry{
		Kind:       ChangelogSkillAdd,
		Tier:       ChangelogTier1,
		Keyword:    "Kubernetes",
		Subsection: "Infrastructure",
	})
}

func TestChangelogEntry_JSONRoundTrip_BulletRewrite(t *testing.T) {
	roundTripChangelogEntry(t, &ChangelogEntry{
		Kind:    ChangelogBulletRewrite,
		Tier:    ChangelogTier2,
		Keyword: "Go",
		Role:    "Senior Engineer",
		Before:  "worked on systems",
		After:   "designed and implemented distributed Go services",
		Source:  RewriteSourceAccomplishmentsDoc,
	})
}

func TestChangelogEntry_JSONRoundTrip_Skip(t *testing.T) {
	roundTripChangelogEntry(t, &ChangelogEntry{
		Kind:    ChangelogSkip,
		Tier:    ChangelogTier1,
		Keyword: "Rust",
		Reason:  SkipReasonNotInSkillsReference,
	})
}

func TestChangelogEntry_JSONRoundTrip_SummaryUpdate(t *testing.T) {
	roundTripChangelogEntry(t, &ChangelogEntry{
		Kind: ChangelogSummaryUpdate,
		Tier: ChangelogSummary,
		Note: "Updated summary to emphasise distributed systems leadership",
	})
}

func TestTailorResult_RoundTrip_Changelog(t *testing.T) {
	original := TailorResult{
		ResumeLabel: "test",
		TierApplied: TierKeyword,
		Changelog: []ChangelogEntry{
			{Kind: ChangelogSkillAdd, Tier: ChangelogTier1, Keyword: "Go"},
		},
		RawChangelog: "## changelog\n- added Go",
	}
	out, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	var decoded TailorResult
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}
	if len(decoded.Changelog) != 1 {
		t.Fatalf("Changelog len = %d, want 1", len(decoded.Changelog))
	}
	if decoded.Changelog[0] != original.Changelog[0] {
		t.Errorf("Changelog[0] = %+v, want %+v", decoded.Changelog[0], original.Changelog[0])
	}
	if decoded.RawChangelog != original.RawChangelog {
		t.Errorf("RawChangelog = %q, want %q", decoded.RawChangelog, original.RawChangelog)
	}
}

func TestTailorResult_Changelog_OmittedWhenEmpty(t *testing.T) {
	r := TailorResult{ResumeLabel: "x", TierApplied: TierNone}
	out, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}
	if bytes.Contains(out, []byte(`"changelog"`)) {
		t.Errorf("expected changelog key absent in JSON; got %s", out)
	}
	if bytes.Contains(out, []byte(`"raw_changelog"`)) {
		t.Errorf("expected raw_changelog key absent in JSON; got %s", out)
	}
}

// --- T011: Validation rejection tests ---

func TestValidateChangelogEntry_Valid_PerKind(t *testing.T) {
	cases := []ChangelogEntry{
		{Kind: ChangelogSkillAdd, Tier: ChangelogTier1, Keyword: "Docker", Subsection: "Infra"},
		{Kind: ChangelogBulletRewrite, Tier: ChangelogTier2, Before: "old", After: "new", Source: RewriteSourceAccomplishmentsDoc},
		{Kind: ChangelogSkip, Tier: ChangelogTier1, Keyword: "Rust", Reason: SkipReasonNoBasisFound},
		{Kind: ChangelogSummaryUpdate, Tier: ChangelogSummary, Note: "updated summary"},
	}
	for _, entry := range cases {
		t.Run(string(entry.Kind), func(t *testing.T) {
			e := entry
			if err := ValidateChangelogEntry(&e); err != nil {
				t.Errorf("ValidateChangelogEntry(%+v) = %v, want nil", e, err)
			}
		})
	}
}

func TestValidateChangelogEntry_Reject_UnknownKind(t *testing.T) {
	e := ChangelogEntry{Kind: "unknown_kind", Tier: ChangelogTier1}
	err := ValidateChangelogEntry(&e)
	if err == nil {
		t.Fatal("expected error for unknown kind, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_changelog") {
		t.Errorf("error %q does not contain 'invalid_changelog'", err.Error())
	}
}

func TestValidateChangelogEntry_Reject_UnknownTier(t *testing.T) {
	e := ChangelogEntry{Kind: ChangelogSkillAdd, Tier: "unknown_tier"}
	err := ValidateChangelogEntry(&e)
	if err == nil {
		t.Fatal("expected error for unknown tier, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_changelog") {
		t.Errorf("error %q does not contain 'invalid_changelog'", err.Error())
	}
}

func TestValidateChangelogEntry_Reject_OversizeKeyword(t *testing.T) {
	e := ChangelogEntry{
		Kind:    ChangelogSkillAdd,
		Tier:    ChangelogTier1,
		Keyword: strings.Repeat("k", 129),
	}
	err := ValidateChangelogEntry(&e)
	if err == nil {
		t.Fatal("expected error for oversize Keyword, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_changelog") {
		t.Errorf("error %q does not contain 'invalid_changelog'", err.Error())
	}
}

func TestValidateChangelogEntry_Reject_OversizeBefore(t *testing.T) {
	e := ChangelogEntry{
		Kind:   ChangelogBulletRewrite,
		Tier:   ChangelogTier2,
		Before: strings.Repeat("x", 2001),
		After:  "new",
		Source: RewriteSourceAccomplishmentsDoc,
	}
	err := ValidateChangelogEntry(&e)
	if err == nil {
		t.Fatal("expected error for oversize Before, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_changelog") {
		t.Errorf("error %q does not contain 'invalid_changelog'", err.Error())
	}
}

func TestValidateChangelogEntry_Reject_ExtraneousField_SkillAdd(t *testing.T) {
	e := ChangelogEntry{
		Kind:    ChangelogSkillAdd,
		Tier:    ChangelogTier1,
		Keyword: "Go",
		Role:    "should-be-empty",
	}
	err := ValidateChangelogEntry(&e)
	if err == nil {
		t.Fatal("expected error for extraneous Role on skill_add, got nil")
	}
	if !strings.Contains(err.Error(), "invalid_changelog") {
		t.Errorf("error %q does not contain 'invalid_changelog'", err.Error())
	}
}
