package storycreator_test

import (
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

// TestCreate_HappyPath verifies a story is written and source_file returned.
func TestCreate_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go\nKubernetes")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Backend Engineer", StartDate: "2020-01", EndDate: "2023-01"}})

	svc := newSvc(t, dir)
	out, err := svc.Create(context.Background(), happyInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.SourceFile == "" {
		t.Error("SourceFile empty")
	}

	// File must exist and contain the SBI content.
	data, readErr := os.ReadFile(filepath.Join(dir, out.SourceFile))
	if readErr != nil {
		t.Fatalf("read written file: %v", readErr)
	}
	content := string(data)
	for _, want := range []string{"Go", "technical", "Backend Engineer", "Situation", "Behavior", "Impact"} {
		if !strings.Contains(content, want) {
			t.Errorf("story file missing %q", want)
		}
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
	if out.SourceFile == "" {
		t.Error("SourceFile empty")
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

// TestCreate_NextAccomplishmentsNumber verifies the file counter increments correctly.
func TestCreate_NextAccomplishmentsNumber(t *testing.T) {
	dir := t.TempDir()
	writeSkills(t, dir, "Go")
	writeCareer(t, dir, []model.ExperienceRef{{Role: "Backend Engineer", StartDate: "2020-01", EndDate: "2023-01"}})

	// Pre-existing accomplishments-0.md and accomplishments-2.md (gap is intentional).
	for _, name := range []string{"accomplishments-0.md", "accomplishments-2.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("existing"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	svc := newSvc(t, dir)
	out, err := svc.Create(context.Background(), happyInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Max existing = 2, so next must be 3.
	if out.SourceFile != "accomplishments-3.md" {
		t.Errorf("SourceFile=%q; want accomplishments-3.md", out.SourceFile)
	}
}
