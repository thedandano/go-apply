package storycreator_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/storycreator"
)

func newSvc(t *testing.T, dir string) *storycreator.Service {
	t.Helper()
	return storycreator.New(dir, fs.NewCareerRepository(), nil)
}

func writeSkills(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "skills.md"), []byte(content), 0o600); err != nil {
		t.Fatalf("write skills.md: %v", err)
	}
}

func writeCareer(t *testing.T, dir string, refs []model.ExperienceRef) {
	t.Helper()
	data, _ := json.Marshal(refs)
	if err := os.WriteFile(filepath.Join(dir, "career.json"), data, 0o600); err != nil {
		t.Fatalf("write career.json: %v", err)
	}
}

func writeAccomplishments(t *testing.T, dir string, acc model.AccomplishmentsJSON) {
	t.Helper()
	data, err := json.Marshal(acc)
	if err != nil {
		t.Fatalf("marshal accomplishments.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "accomplishments.json"), data, 0o600); err != nil {
		t.Fatalf("write accomplishments.json: %v", err)
	}
}

func readAccomplishments(t *testing.T, dir string) model.AccomplishmentsJSON {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "accomplishments.json"))
	if err != nil {
		t.Fatalf("read accomplishments.json: %v", err)
	}
	var acc model.AccomplishmentsJSON
	if err := json.Unmarshal(data, &acc); err != nil {
		t.Fatalf("parse accomplishments.json: %v", err)
	}
	return acc
}

func happyInput() model.StoryInput {
	return model.StoryInput{
		PrimarySkill: "Go",
		StoryType:    model.StoryTypeTechnical,
		JobTitle:     "Backend Engineer",
		IsNewJob:     false,
		Situation:    "The team needed a faster API",
		Behavior:     "I rewrote the handler in Go",
		Impact:       "Latency dropped by 40%",
	}
}

// TestCreate_HappyPath verifies a story is written to accomplishments.json and StoryID returned.
func TestCreate_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go\nKubernetes")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Backend Engineer", StartDate: "2020-01", EndDate: "2023-01"}})

	svc := newSvc(t, dir)
	out, err := svc.Create(context.Background(), happyInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.StoryID == "" {
		t.Error("StoryID empty")
	}

	// accomplishments.json must exist and contain the SBI content.
	acc := readAccomplishments(t, dir)
	if len(acc.CreatedStories) != 1 {
		t.Fatalf("created_stories length = %d; want 1", len(acc.CreatedStories))
	}
	text := acc.CreatedStories[0].Text
	for _, want := range []string{"Situation", "Behavior", "Impact"} {
		if !strings.Contains(text, want) {
			t.Errorf("story text missing %q", want)
		}
	}

	// No .md files must be written.
	matches, _ := filepath.Glob(filepath.Join(dir, "accomplishments-*.md"))
	if len(matches) != 0 {
		t.Errorf("unexpected .md files written: %v", matches)
	}
}

// TestCreate_SkillNotFound expects an error when primary skill is not in skills.md.
func TestCreate_SkillNotFound(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Java\nKubernetes")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Backend Engineer", StartDate: "2020-01", EndDate: "2023-01"}})

	svc := newSvc(t, dir)
	inp := happyInput()
	inp.PrimarySkill = "Go" // not in skills.md

	_, err := svc.Create(context.Background(), inp)
	if err == nil {
		t.Fatal("expected error for unknown skill, got nil")
	}
	if !strings.Contains(err.Error(), "skill not found") {
		t.Errorf("error %q does not mention 'skill not found'", err.Error())
	}
}

// TestCreate_EmptyBehavior expects an error when a required SBI field is blank.
func TestCreate_EmptyBehavior(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Backend Engineer", StartDate: "2020-01", EndDate: "2023-01"}})

	svc := newSvc(t, dir)
	inp := happyInput()
	inp.Behavior = "   "

	_, err := svc.Create(context.Background(), inp)
	if err == nil {
		t.Fatal("expected error for empty behavior, got nil")
	}
	if !strings.Contains(err.Error(), "empty field: behavior") {
		t.Errorf("error %q does not mention 'empty field: behavior'", err.Error())
	}
}

// TestCreate_TitleNotFound expects an error when job_title is absent and is_new_job=false.
func TestCreate_TitleNotFound(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Other Role", StartDate: "2020-01", EndDate: "2021-01"}})

	svc := newSvc(t, dir)
	inp := happyInput()
	inp.IsNewJob = false

	_, err := svc.Create(context.Background(), inp)
	if err == nil {
		t.Fatal("expected error for unknown job title, got nil")
	}
	if !strings.Contains(err.Error(), "job title not found") {
		t.Errorf("error %q does not mention 'job title not found'", err.Error())
	}
}

// TestCreate_IsNewJob_AppendsCareer verifies career.json is updated when is_new_job=true.
func TestCreate_IsNewJob_AppendsCareer(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go")
	// career.json absent — is_new_job=true should create it.

	svc := newSvc(t, dir)
	inp := happyInput()
	inp.IsNewJob = true
	inp.StartDate = "2022-01"
	inp.EndDate = "present"

	out, err := svc.Create(context.Background(), inp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.StoryID == "" {
		t.Error("StoryID empty")
	}

	// career.json must now contain the new role.
	raw, readErr := os.ReadFile(filepath.Join(dir, "career.json"))
	if readErr != nil {
		t.Fatalf("read career.json: %v", readErr)
	}
	var refs []model.ExperienceRef
	if jsonErr := json.Unmarshal(raw, &refs); jsonErr != nil {
		t.Fatalf("parse career.json: %v", jsonErr)
	}
	found := false
	for _, r := range refs {
		if strings.EqualFold(r.Role, "Backend Engineer") {
			found = true
			if r.StartDate != "2022-01" {
				t.Errorf("start_date=%q; want 2022-01", r.StartDate)
			}
		}
	}
	if !found {
		t.Error("Backend Engineer not found in career.json after is_new_job=true")
	}
}

// TestCreate_NextAccomplishmentsNumber verifies the ID counter increments past gaps correctly.
func TestCreate_NextAccomplishmentsNumber(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Backend Engineer", StartDate: "2020-01", EndDate: "2023-01"}})

	// Pre-existing accomplishments with ids "0" and "2" (gap is intentional).
	writeAccomplishments(t, dir, model.AccomplishmentsJSON{
		SchemaVersion: "1",
		CreatedStories: []model.CreatedStory{
			{ID: "0", Skill: "Go", Type: model.StoryTypeTechnical, JobTitle: "Backend Engineer", Text: "old story 0"},
			{ID: "2", Skill: "Go", Type: model.StoryTypeTechnical, JobTitle: "Backend Engineer", Text: "old story 2"},
		},
	})

	svc := newSvc(t, dir)
	out, err := svc.Create(context.Background(), happyInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Max existing = 2, so next must be 3.
	if out.StoryID != "3" {
		t.Errorf("StoryID=%q; want \"3\"", out.StoryID)
	}

	// Verify the entry was appended with the correct ID.
	acc := readAccomplishments(t, dir)
	last := acc.CreatedStories[len(acc.CreatedStories)-1]
	if last.ID != "3" {
		t.Errorf("last created_story id=%q; want \"3\"", last.ID)
	}

	// No .md files must be written.
	matches, _ := filepath.Glob(filepath.Join(dir, "accomplishments-*.md"))
	if len(matches) != 0 {
		t.Errorf("unexpected .md files written: %v", matches)
	}
}

// TestCreate_MissingAccomplishments verifies that a missing accomplishments.json yields first id "0".
func TestCreate_MissingAccomplishments(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Backend Engineer", StartDate: "2020-01", EndDate: "2023-01"}})
	// accomplishments.json intentionally absent.

	svc := newSvc(t, dir)
	out, err := svc.Create(context.Background(), happyInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.StoryID != "0" {
		t.Errorf("StoryID=%q; want \"0\"", out.StoryID)
	}

	acc := readAccomplishments(t, dir)
	if len(acc.CreatedStories) != 1 {
		t.Fatalf("created_stories length = %d; want 1", len(acc.CreatedStories))
	}
	if acc.CreatedStories[0].ID != "0" {
		t.Errorf("created_stories[0].id=%q; want \"0\"", acc.CreatedStories[0].ID)
	}
}

// TestCreate_UnsupportedSchemaVersion verifies that an unsupported schema_version returns an error
// and does NOT modify the file.
func TestCreate_UnsupportedSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Backend Engineer", StartDate: "2020-01", EndDate: "2023-01"}})

	badPath := filepath.Join(dir, "accomplishments.json")
	original, _ := json.Marshal(model.AccomplishmentsJSON{SchemaVersion: "99"})
	if err := os.WriteFile(badPath, original, 0o600); err != nil {
		t.Fatalf("write v99 accomplishments.json: %v", err)
	}

	svc := newSvc(t, dir)
	_, err := svc.Create(context.Background(), happyInput())
	if err == nil {
		t.Fatal("expected error for unsupported schema_version, got nil")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("error %q does not mention 'schema_version'", err.Error())
	}

	after, readErr := os.ReadFile(badPath)
	if readErr != nil {
		t.Fatalf("read accomplishments.json after error: %v", readErr)
	}
	if !bytes.Equal(after, original) {
		t.Errorf("accomplishments.json was modified despite error")
	}
}

// TestCreate_CorruptAccomplishments verifies that a corrupt accomplishments.json returns an error
// and does NOT modify the file.
func TestCreate_CorruptAccomplishments(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Backend Engineer", StartDate: "2020-01", EndDate: "2023-01"}})

	// Write corrupt JSON.
	corruptPath := filepath.Join(dir, "accomplishments.json")
	original := []byte("not-json")
	if err := os.WriteFile(corruptPath, original, 0o600); err != nil {
		t.Fatalf("write corrupt accomplishments.json: %v", err)
	}

	svc := newSvc(t, dir)
	_, err := svc.Create(context.Background(), happyInput())
	if err == nil {
		t.Fatal("expected error for corrupt accomplishments.json, got nil")
	}
	if !strings.Contains(err.Error(), "parse accomplishments") {
		t.Errorf("error %q does not mention 'parse accomplishments'", err.Error())
	}

	// File must be unchanged.
	after, readErr := os.ReadFile(corruptPath)
	if readErr != nil {
		t.Fatalf("read accomplishments.json after error: %v", readErr)
	}
	if !bytes.Equal(after, original) {
		t.Errorf("accomplishments.json was modified despite error; got %q; want %q", after, original)
	}
}
