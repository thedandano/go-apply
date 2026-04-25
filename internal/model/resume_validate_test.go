package model_test

import (
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
)

// T003: knownSections allowlist must include all 6 Tier 4 keys.
// This test must FAIL at runtime until T006 adds the keys to knownSections.
func TestKnownSections_Tier4Keys(t *testing.T) {
	validContact := model.ContactInfo{Name: "Alice Smith"}
	validExperience := []model.ExperienceEntry{
		{Company: "Acme Corp", Role: "Engineer", StartDate: "2020-01", Bullets: []string{}},
	}
	tier4Keys := []string{"languages", "speaking", "open_source", "patents", "interests", "references"}
	sm := &model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       validContact,
		Experience:    validExperience,
		Order:         tier4Keys,
	}
	if err := model.ValidateSectionMap(sm); err != nil {
		t.Errorf("all Tier 4 keys must be valid in SectionMap.Order; got error: %v", err)
	}
}

func TestValidateSectionMap(t *testing.T) {
	validContact := model.ContactInfo{Name: "Alice Smith"}
	validExperience := []model.ExperienceEntry{
		{
			Company:   "Acme Corp",
			Role:      "Engineer",
			StartDate: "2020-01",
			Bullets:   []string{},
		},
	}

	tests := []struct {
		name        string
		input       *model.SectionMap
		wantErr     bool
		errContains string
	}{
		{
			name: "valid minimal SectionMap",
			input: &model.SectionMap{
				SchemaVersion: model.CurrentSchemaVersion,
				Contact:       validContact,
				Experience:    validExperience,
			},
			wantErr: false,
		},
		{
			name: "missing contact.name",
			input: &model.SectionMap{
				SchemaVersion: model.CurrentSchemaVersion,
				Contact:       model.ContactInfo{Name: ""},
				Experience:    validExperience,
			},
			wantErr:     true,
			errContains: "contact.name",
		},
		{
			name: "missing experience (nil)",
			input: &model.SectionMap{
				SchemaVersion: model.CurrentSchemaVersion,
				Contact:       validContact,
				Experience:    nil,
			},
			wantErr: true,
		},
		{
			name: "wrong schema version (0)",
			input: &model.SectionMap{
				SchemaVersion: 0,
				Contact:       validContact,
				Experience:    validExperience,
			},
			wantErr:     true,
			errContains: "schema",
		},
		{
			name: "unknown section key in Order",
			input: &model.SectionMap{
				SchemaVersion: model.CurrentSchemaVersion,
				Contact:       validContact,
				Experience:    validExperience,
				Order:         []string{"contact", "experience", "unknown_section_xyz"},
			},
			wantErr: true,
		},
		{
			name: "skills kind flat with Categorized set (kind mismatch)",
			input: &model.SectionMap{
				SchemaVersion: model.CurrentSchemaVersion,
				Contact:       validContact,
				Experience:    validExperience,
				Skills: &model.SkillsSection{
					Kind:        model.SkillsKindFlat,
					Categorized: map[string][]string{"Languages": {"Go", "Python"}},
				},
			},
			wantErr: true,
		},
		{
			name: "skills kind categorized with Flat set (kind mismatch)",
			input: &model.SectionMap{
				SchemaVersion: model.CurrentSchemaVersion,
				Contact:       validContact,
				Experience:    validExperience,
				Skills: &model.SkillsSection{
					Kind: model.SkillsKindCategorized,
					Flat: "Go, Python",
				},
			},
			wantErr: true,
		},
		{
			name: "malformed start_date in experience entry (invalid month 13)",
			input: &model.SectionMap{
				SchemaVersion: model.CurrentSchemaVersion,
				Contact:       validContact,
				Experience: []model.ExperienceEntry{
					{
						Company:   "Bad Dates Inc",
						Role:      "Engineer",
						StartDate: "2024-13",
						Bullets:   []string{},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := model.ValidateSectionMap(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tc.errContains != "" && err != nil && !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.errContains)
			}
		})
	}
}
