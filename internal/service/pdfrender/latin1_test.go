package pdfrender

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
)

// captureHandler collects slog records for test assertions.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) warnRecords() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []slog.Record
	for _, r := range h.records {
		if r.Level == slog.LevelWarn {
			out = append(out, r)
		}
	}
	return out
}

func (h *captureHandler) attrCount(r slog.Record) int {
	count := 0
	r.Attrs(func(_ slog.Attr) bool {
		count++
		return true
	})
	return count
}

// TestTransliterateField_FR002Mappings covers every explicit FR-002 mapping.
func TestTransliterateField_FR002Mappings(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		// Punctuation / typographic characters
		{"em-dash U+2014", "—", "-"},
		{"en-dash U+2013", "–", "-"},
		{"bullet U+2022", "•", "-"},
		{"left-single-quote U+2018", "‘", "'"},
		{"right-single-quote U+2019", "’", "'"},
		{"left-double-quote U+201C", "“", `"`},
		{"right-double-quote U+201D", "”", `"`},
		{"ellipsis U+2026", "…", "..."},
		{"non-breaking-space U+00A0", " ", " "},
		{"hyphen U+2010", "‐", "-"},
		{"non-breaking-hyphen U+2011", "‑", "-"},
		// Accented Latin via NFD decomposition
		{"e-acute U+00E9", "é", "e"},
		{"u-umlaut U+00FC", "ü", "u"},
		{"n-tilde U+00F1", "ñ", "n"},
		{"a-grave U+00E0", "à", "a"},
		{"o-umlaut U+00F6", "ö", "o"},
		// Explicit non-NFD mappings
		{"eszett U+00DF", "ß", "ss"},
		// Mixed ASCII stays unchanged
		{"pure ASCII", "hello", "hello"},
		{"empty string", "", ""},
		// Embedded mixed
		{"mixed accented", "José", "Jose"},
		{"mixed em-dash", "foo—bar", "foo-bar"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := transliterateField(tc.input, "test.field")
			if got != tc.want {
				t.Errorf("transliterateField(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestTransliterateField_UnknownCodepoints checks that truly unrepresentable chars (>0xFF,
// not in map, no NFD base) become '?' and emit a slog.Warn with exactly 2 attrs.
func TestTransliterateField_UnknownCodepoints(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantRune rune
	}{
		{"snowman U+2603", "☃", '?'},
		{"emoji U+1F600", "\U0001F600", '?'},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &captureHandler{}
			orig := slog.Default()
			slog.SetDefault(slog.New(h))
			defer slog.SetDefault(orig)

			got := transliterateField(tc.input, "contact.name")

			if got != string(tc.wantRune) {
				t.Errorf("transliterateField(%q) = %q, want %q", tc.input, got, string(tc.wantRune))
			}

			warns := h.warnRecords()
			if len(warns) != 1 {
				t.Fatalf("expected 1 warn record, got %d", len(warns))
			}

			// PII guard: exactly 2 attrs (codepoint + field), no surrounding text.
			count := h.attrCount(warns[0])
			if count != 2 {
				t.Errorf("warn log must have exactly 2 attrs (codepoint + field), got %d", count)
			}
		})
	}
}

// TestTransliterateLatin1_DeepCopy verifies original SectionMap is not mutated.
func TestTransliterateLatin1_DeepCopy(t *testing.T) {
	orig := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact: model.ContactInfo{
			Name: "José Müñoz",
		},
		Summary: "“Hello World”",
	}

	origName := orig.Contact.Name
	origSummary := orig.Summary

	result := transliterateLatin1(orig)

	// Original must be unchanged
	if orig.Contact.Name != origName {
		t.Errorf("original Contact.Name mutated: got %q", orig.Contact.Name)
	}
	if orig.Summary != origSummary {
		t.Errorf("original Summary mutated: got %q", orig.Summary)
	}

	// Result must be transliterated
	if result.Contact.Name == origName {
		t.Errorf("result.Contact.Name not transliterated: got %q", result.Contact.Name)
	}
	if result.Summary == origSummary {
		t.Errorf("result.Summary not transliterated: got %q", result.Summary)
	}
}

// TestTransliterateLatin1_FullSectionMap checks transliteration across all relevant fields.
func TestTransliterateLatin1_FullSectionMap(t *testing.T) {
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Order:         []string{"contact", "summary", "experience", "skills"},
		Contact: model.ContactInfo{
			Name:     "José Müller", // é→e, ü→u
			Email:    "jose@example.com",
			Phone:    "555-0100",
			Location: "Münich, Germany",                // ü→u
			Links:    []string{"linkedin.com/in/josé"}, // é→e
		},
		Summary: "“Seasoned engineer”", // "…" → "…"
		Experience: []model.ExperienceEntry{
			{
				Company:   "Acme GmbH",
				Role:      "Engineer",
				StartDate: "2020-01",
				Bullets: []string{
					"Led team using en–dash approach", // en-dash → -
				},
			},
		},
		Skills: &model.SkillsSection{
			Kind: model.SkillsKindFlat,
			Flat: "• Python • Go", // bullet → -
		},
	}

	result := transliterateLatin1(sm)

	// Contact
	if result.Contact.Name != "Jose Muller" {
		t.Errorf("Contact.Name: got %q, want %q", result.Contact.Name, "Jose Muller")
	}
	if result.Contact.Location != "Munich, Germany" {
		t.Errorf("Contact.Location: got %q, want %q", result.Contact.Location, "Munich, Germany")
	}
	if len(result.Contact.Links) != 1 || result.Contact.Links[0] != "linkedin.com/in/jose" {
		t.Errorf("Contact.Links[0]: got %q, want %q", result.Contact.Links[0], "linkedin.com/in/jose")
	}

	// Summary (smart quotes)
	if result.Summary != `"Seasoned engineer"` {
		t.Errorf("Summary: got %q, want %q", result.Summary, `"Seasoned engineer"`)
	}

	// Experience bullet (en-dash)
	if len(result.Experience) == 0 || len(result.Experience[0].Bullets) == 0 {
		t.Fatal("expected experience with bullet")
	}
	got := result.Experience[0].Bullets[0]
	want := "Led team using en-dash approach"
	if got != want {
		t.Errorf("Experience bullet: got %q, want %q", got, want)
	}

	// Skills (bullet)
	if result.Skills == nil {
		t.Fatal("expected non-nil Skills")
	}
	if result.Skills.Flat != "- Python - Go" {
		t.Errorf("Skills.Flat: got %q, want %q", result.Skills.Flat, "- Python - Go")
	}

	// Order deep copy
	if len(result.Order) != len(sm.Order) {
		t.Errorf("Order length: got %d, want %d", len(result.Order), len(sm.Order))
	}
}

// TestTransliterateLatin1_SmartQuotesInSummary checks smart-quote mapping.
func TestTransliterateLatin1_SmartQuotesInSummary(t *testing.T) {
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Test"},
		Summary:       "“Smart” and ‘single’",
	}
	result := transliterateLatin1(sm)
	want := `"Smart" and 'single'`
	if result.Summary != want {
		t.Errorf("Summary: got %q, want %q", result.Summary, want)
	}
}
