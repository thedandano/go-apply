package model

import (
	"bytes"
	"encoding/json"
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
