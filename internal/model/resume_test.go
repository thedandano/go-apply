package model_test

import (
	"encoding/json"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
)

// T002: JSON round-trip tests for all 6 Tier 4 entry structs.
// These tests must FAIL (compile error) until T005 adds the structs.
func TestTier4EntryStructs_JSONRoundTrip(t *testing.T) {
	t.Run("LanguageEntry", func(t *testing.T) {
		orig := model.LanguageEntry{Name: "Go", Proficiency: "Fluent"}
		b, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got model.LanguageEntry
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got != orig {
			t.Errorf("round-trip: got %+v, want %+v", got, orig)
		}
	})
	t.Run("SpeakingEntry", func(t *testing.T) {
		orig := model.SpeakingEntry{Title: "GopherCon 2023", Event: "GopherCon", Date: "2023-07", URL: "https://example.com"}
		b, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got model.SpeakingEntry
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got != orig {
			t.Errorf("round-trip: got %+v, want %+v", got, orig)
		}
	})
	t.Run("OpenSourceEntry", func(t *testing.T) {
		orig := model.OpenSourceEntry{Project: "go-apply", Role: "Maintainer", URL: "https://github.com/x/y", Description: "Job application CLI"}
		b, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got model.OpenSourceEntry
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got != orig {
			t.Errorf("round-trip: got %+v, want %+v", got, orig)
		}
	})
	t.Run("PatentEntry", func(t *testing.T) {
		orig := model.PatentEntry{Title: "Fast Algo", Number: "US12345678", Date: "2022-01", URL: "https://patents.google.com/x"}
		b, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got model.PatentEntry
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got != orig {
			t.Errorf("round-trip: got %+v, want %+v", got, orig)
		}
	})
	t.Run("InterestEntry", func(t *testing.T) {
		orig := model.InterestEntry{Name: "Distributed systems"}
		b, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got model.InterestEntry
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got != orig {
			t.Errorf("round-trip: got %+v, want %+v", got, orig)
		}
	})
	t.Run("ReferenceEntry", func(t *testing.T) {
		orig := model.ReferenceEntry{Name: "Jane Doe", Title: "VP Engineering", Company: "Acme", Contact: "jane@acme.com"}
		b, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got model.ReferenceEntry
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got != orig {
			t.Errorf("round-trip: got %+v, want %+v", got, orig)
		}
	})
	t.Run("ReferenceEntry_available_upon_request", func(t *testing.T) {
		orig := model.ReferenceEntry{Name: "Available upon request"}
		b, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var got model.ReferenceEntry
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got != orig {
			t.Errorf("round-trip: got %+v, want %+v", got, orig)
		}
	})
	t.Run("SectionMap_Tier4_roundtrip", func(t *testing.T) {
		orig := model.SectionMap{
			SchemaVersion: model.CurrentSchemaVersion,
			Contact:       model.ContactInfo{Name: "Alice"},
			Experience:    []model.ExperienceEntry{{Company: "Acme", Role: "Eng", StartDate: "2020-01", Bullets: []string{}}},
			Languages:     []model.LanguageEntry{{Name: "Go", Proficiency: "Fluent"}},
			Speaking:      []model.SpeakingEntry{{Title: "Talk A", Event: "Conf", Date: "2023"}},
			OpenSource:    []model.OpenSourceEntry{{Project: "go-apply", Role: "Author"}},
			Patents:       []model.PatentEntry{{Title: "Algorithm", Number: "US123"}},
			Interests:     []model.InterestEntry{{Name: "OSS"}},
			References:    []model.ReferenceEntry{{Name: "Available upon request"}},
		}
		b, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("marshal SectionMap: %v", err)
		}
		var got model.SectionMap
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal SectionMap: %v", err)
		}
		if len(got.Languages) != 1 || got.Languages[0].Name != "Go" {
			t.Errorf("Languages round-trip: got %v", got.Languages)
		}
		if len(got.Speaking) != 1 || got.Speaking[0].Title != "Talk A" {
			t.Errorf("Speaking round-trip: got %v", got.Speaking)
		}
		if len(got.OpenSource) != 1 || got.OpenSource[0].Project != "go-apply" {
			t.Errorf("OpenSource round-trip: got %v", got.OpenSource)
		}
		if len(got.Patents) != 1 || got.Patents[0].Title != "Algorithm" {
			t.Errorf("Patents round-trip: got %v", got.Patents)
		}
		if len(got.Interests) != 1 || got.Interests[0].Name != "OSS" {
			t.Errorf("Interests round-trip: got %v", got.Interests)
		}
		if len(got.References) != 1 || got.References[0].Name != "Available upon request" {
			t.Errorf("References round-trip: got %v", got.References)
		}
	})
}

func TestExperienceEntry_BulletID(t *testing.T) {
	e := model.ExperienceEntry{}
	tests := []struct {
		entryIdx  int
		bulletIdx int
		want      string
	}{
		{0, 0, "exp-0-b0"},
		{0, 2, "exp-0-b2"},
		{1, 0, "exp-1-b0"},
		{3, 5, "exp-3-b5"},
	}
	for _, tc := range tests {
		got := e.BulletID(tc.entryIdx, tc.bulletIdx)
		if got != tc.want {
			t.Errorf("BulletID(%d,%d) = %q, want %q", tc.entryIdx, tc.bulletIdx, got, tc.want)
		}
	}
}

func TestExperienceEntry_ID(t *testing.T) {
	e := model.ExperienceEntry{}
	tests := []struct {
		idx  int
		want string
	}{
		{0, "exp-0"},
		{1, "exp-1"},
		{5, "exp-5"},
	}
	for _, tc := range tests {
		got := e.ID(tc.idx)
		if got != tc.want {
			t.Errorf("ID(%d) = %q, want %q", tc.idx, got, tc.want)
		}
	}
}
