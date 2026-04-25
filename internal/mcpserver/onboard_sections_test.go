package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
)

func validSectionMap() model.SectionMap {
	return model.SectionMap{
		SchemaVersion: model.CurrentSchemaVersion,
		Contact:       model.ContactInfo{Name: "Alice Smith"},
		Experience: []model.ExperienceEntry{
			{Company: "Acme Corp", Role: "Engineer", StartDate: "2020-01", Bullets: []string{}},
		},
	}
}

func TestOnboardSections(t *testing.T) {
	t.Run("sections_field_present_parses_and_validates", func(t *testing.T) {
		sm := validSectionMap()
		data, err := json.Marshal(sm)
		if err != nil {
			t.Fatalf("marshal SectionMap: %v", err)
		}
		var parsed model.SectionMap
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("unmarshal SectionMap: %v", err)
		}
		if err := model.ValidateSectionMap(&parsed); err != nil {
			t.Errorf("ValidateSectionMap on round-tripped sections: %v", err)
		}
	})

	t.Run("missing_contact_name_produces_schema_error", func(t *testing.T) {
		sm := validSectionMap()
		sm.Contact.Name = ""

		err := model.ValidateSectionMap(&sm)
		if err == nil {
			t.Fatal("expected SchemaError for missing contact.name, got nil")
		}
		var se model.SchemaError
		if !errors.As(err, &se) {
			t.Fatalf("expected model.SchemaError, got %T: %v", err, err)
		}
		if se.Field != "contact.name" {
			t.Errorf("SchemaError.Field = %q, want %q", se.Field, "contact.name")
		}
	})

	t.Run("missing_experience_produces_schema_error", func(t *testing.T) {
		sm := validSectionMap()
		sm.Experience = nil

		err := model.ValidateSectionMap(&sm)
		if err == nil {
			t.Fatal("expected SchemaError for nil experience, got nil")
		}
		var se model.SchemaError
		if !errors.As(err, &se) {
			t.Fatalf("expected model.SchemaError, got %T: %v", err, err)
		}
		if se.Field != "experience" {
			t.Errorf("SchemaError.Field = %q, want %q", se.Field, "experience")
		}
	})

	t.Run("wrong_schema_version_produces_unsupported_error", func(t *testing.T) {
		sm := validSectionMap()
		sm.SchemaVersion = 0

		err := model.ValidateSectionMap(&sm)
		if err == nil {
			t.Fatal("expected error for schema_version 0, got nil")
		}
		if !errors.Is(err, model.ErrSchemaVersionUnsupported) {
			t.Errorf("expected errors.Is(err, ErrSchemaVersionUnsupported), got: %v", err)
		}
	})

	// T004: parseSectionsArg must accept all 6 Tier 4 section keys.
	// This test must FAIL (compile error) until T005 adds Tier 4 fields to model.SectionMap.
	t.Run("tier4_keys_accepted_by_handler", func(t *testing.T) {
		sm := model.SectionMap{
			SchemaVersion: model.CurrentSchemaVersion,
			Contact:       model.ContactInfo{Name: "Alice Smith"},
			Experience:    []model.ExperienceEntry{{Company: "Acme", Role: "Eng", StartDate: "2020-01", Bullets: []string{}}},
			Languages:     []model.LanguageEntry{{Name: "Go", Proficiency: "Fluent"}},
			Speaking:      []model.SpeakingEntry{{Title: "Talk A", Event: "Conf", Date: "2023"}},
			OpenSource:    []model.OpenSourceEntry{{Project: "go-apply", Role: "Author"}},
			Patents:       []model.PatentEntry{{Title: "Algorithm", Number: "US123"}},
			Interests:     []model.InterestEntry{{Name: "OSS"}},
			References:    []model.ReferenceEntry{{Name: "Available upon request"}},
		}
		data, err := json.Marshal(sm)
		if err != nil {
			t.Fatalf("marshal SectionMap: %v", err)
		}
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "add_resume",
				Arguments: map[string]any{
					"resume_content": "Engineer with Go experience.",
					"resume_label":   "backend",
					"sections":       string(data),
				},
			},
		}
		result := mcpserver.HandleAddResume(context.Background(), &req, &stubOnboarder{})
		text := extractText(t, result)
		var response map[string]any
		if err := json.Unmarshal([]byte(text), &response); err != nil {
			t.Fatalf("result is not valid JSON: %v\ntext: %s", err, text)
		}
		if _, hasErr := response["error"]; hasErr {
			t.Errorf("handler rejected Tier 4 section keys: %s", text)
		}
	})

	t.Run("sections_omitted_handler_succeeds", func(t *testing.T) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name: "onboard_user",
				Arguments: map[string]any{
					"resume_content": "Engineer with 5 years of Go experience.",
					"resume_label":   "backend",
				},
			},
		}
		result := mcpserver.HandleOnboardUser(context.Background(), &req, &stubOnboarder{})
		text := extractText(t, result)

		var response map[string]any
		if err := json.Unmarshal([]byte(text), &response); err != nil {
			t.Fatalf("result is not valid JSON: %v\ntext: %s", err, text)
		}
		if _, hasErr := response["error"]; hasErr {
			t.Errorf("handler returned error when sections omitted: %s", text)
		}
	})
}
