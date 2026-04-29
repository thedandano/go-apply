package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
)

// writeCompiledProfile writes a CompiledProfile as profile-compiled.json in dir.
//
//nolint:gocritic // hugeParam: test helper, CompiledProfile passed by value for clarity
func writeCompiledProfile(t *testing.T, dir string, p model.CompiledProfile) {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile-compiled.json"), data, 0o600); err != nil {
		t.Fatalf("write compiled profile: %v", err)
	}
}

func assertStringKey(t *testing.T, m map[string]interface{}, key, want string) {
	t.Helper()
	got, ok := m[key].(string)
	if !ok {
		t.Errorf("key %q missing or not string in response", key)
		return
	}
	if got != want {
		t.Errorf("%s=%q; want %q", key, got, want)
	}
}

// TestHandleCompileProfileWith_EmptyInput verifies compilation with no prior and no input
// returns status "compiled" with empty orphans.
func TestHandleCompileProfileWith_EmptyInput(t *testing.T) {
	dir := t.TempDir()
	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, model.AssembleInput{})
	if result == nil {
		t.Fatal("nil result")
	}
	text := extractText(t, result)
	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, text)
	}
	assertStringKey(t, resp, "status", "compiled")
	orphans, _ := resp["orphaned_skills"].([]interface{})
	if len(orphans) != 0 {
		t.Errorf("orphaned_skills = %v; want empty", orphans)
	}
}

// TestHandleCompileProfileWith_OrphanOutput verifies skills with no story coverage appear
// in orphaned_skills.
func TestHandleCompileProfileWith_OrphanOutput(t *testing.T) {
	dir := t.TempDir()
	input := model.AssembleInput{
		Skills: []string{"Go", "Kubernetes", "Terraform"},
		Stories: []model.AssembleStory{
			{Accomplishment: "Go work", Tags: []string{"Go"}},
		},
	}
	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, input)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, text)
	}

	assertStringKey(t, resp, "status", "compiled")

	orphans, _ := resp["orphaned_skills"].([]interface{})
	if len(orphans) != 2 {
		t.Errorf("orphaned_skills len = %d; want 2 (Kubernetes, Terraform)", len(orphans))
	}
}

// TestHandleCompileProfileWith_IDResolutionError verifies that an unknown story ID returns
// an error response (not a panic or "compiled" status).
func TestHandleCompileProfileWith_IDResolutionError(t *testing.T) {
	dir := t.TempDir()
	prior := model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now().Add(-time.Minute),
		Stories:       []model.Story{{ID: "story-001", Text: "first story"}},
	}
	writeCompiledProfile(t, dir, prior)

	input := model.AssembleInput{
		Stories: []model.AssembleStory{
			{ID: "story-999", Tags: []string{"Go"}}, // unknown ID
		},
	}

	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, input)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, text)
	}

	if _, ok := resp["error"]; !ok {
		t.Error("expected error key when story ID cannot be resolved")
	}
	if resp["status"] == "compiled" {
		t.Error("status must not be 'compiled' when ID resolution fails")
	}
}

// TestHandleCompileProfileWith_SkillUnion verifies effective_skills = prior ∪ input - removes,
// and that skills_added / skills_removed are correctly computed.
func TestHandleCompileProfileWith_SkillUnion(t *testing.T) {
	dir := t.TempDir()
	prior := model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now().Add(-time.Minute),
		Skills:        []string{"Go", "Kubernetes"},
	}
	writeCompiledProfile(t, dir, prior)

	input := model.AssembleInput{
		Skills:       []string{"Terraform"},
		RemoveSkills: []string{"Kubernetes"},
		Stories: []model.AssembleStory{
			{Accomplishment: "Terraform and Go work", Tags: []string{"Terraform", "Go"}},
		},
	}

	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, input)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, text)
	}

	assertStringKey(t, resp, "status", "compiled")

	skillsAdded, _ := resp["skills_added"].([]interface{})
	if len(skillsAdded) != 1 || skillsAdded[0].(string) != "Terraform" {
		t.Errorf("skills_added = %v; want [Terraform]", skillsAdded)
	}

	skillsRemoved, _ := resp["skills_removed"].([]interface{})
	if len(skillsRemoved) != 1 || skillsRemoved[0].(string) != "Kubernetes" {
		t.Errorf("skills_removed = %v; want [Kubernetes]", skillsRemoved)
	}

	// Effective skills = Go + Terraform; both covered by the story → no orphans.
	orphans, _ := resp["orphaned_skills"].([]interface{})
	if len(orphans) != 0 {
		t.Errorf("orphaned_skills = %v; want empty", orphans)
	}
}

// TestHandleCompileProfileWith_RichDiff_CoverageGained verifies that a skill previously
// orphaned in the prior profile and now covered appears in coverage_gained.
func TestHandleCompileProfileWith_RichDiff_CoverageGained(t *testing.T) {
	dir := t.TempDir()
	prior := model.CompiledProfile{
		SchemaVersion:  "1",
		CompiledAt:     time.Now().Add(-time.Minute),
		Skills:         []string{"Go", "Kubernetes"},
		OrphanedSkills: []model.OrphanedSkill{{Skill: "Kubernetes", Deferred: false}},
		Stories:        []model.Story{{ID: "story-001", Text: "Go work", Skills: []string{"Go"}}},
	}
	writeCompiledProfile(t, dir, prior)

	// New story covers Kubernetes — removing it from orphans.
	input := model.AssembleInput{
		Stories: []model.AssembleStory{
			{Accomplishment: "K8s migration", Tags: []string{"Kubernetes", "Go"}},
		},
	}

	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, input)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, text)
	}

	coverageGained, _ := resp["coverage_gained"].([]interface{})
	found := false
	for _, c := range coverageGained {
		if c.(string) == "Kubernetes" {
			found = true
		}
	}
	if !found {
		t.Errorf("coverage_gained = %v; want Kubernetes", coverageGained)
	}

	// Kubernetes should no longer be orphaned.
	orphans, _ := resp["orphaned_skills"].([]interface{})
	for _, o := range orphans {
		if om, ok := o.(map[string]interface{}); ok {
			if om["skill"].(string) == "Kubernetes" {
				t.Error("Kubernetes still in orphaned_skills after being covered")
			}
		}
	}
}

// TestHandleCompileProfileWith_StoryCounts verifies stories_added and stories_updated.
func TestHandleCompileProfileWith_StoryCounts(t *testing.T) {
	dir := t.TempDir()
	prior := model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now().Add(-time.Minute),
		Skills:        []string{"Go", "Kubernetes"},
		Stories:       []model.Story{{ID: "story-001", Text: "existing", Skills: []string{"Go"}}},
	}
	writeCompiledProfile(t, dir, prior)

	input := model.AssembleInput{
		Stories: []model.AssembleStory{
			{ID: "story-001", Tags: []string{"Go"}},                    // updated
			{Accomplishment: "K8s work", Tags: []string{"Kubernetes"}}, // added
		},
	}

	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, input)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, text)
	}

	storiesAdded, _ := resp["stories_added"].(float64)
	storiesUpdated, _ := resp["stories_updated"].(float64)
	if storiesAdded != 1 {
		t.Errorf("stories_added = %v; want 1", storiesAdded)
	}
	if storiesUpdated != 1 {
		t.Errorf("stories_updated = %v; want 1", storiesUpdated)
	}
}

// TestHandleCompileProfile_InvalidSkillsJSON verifies that invalid JSON in the skills arg
// returns an error response from the JSON parse layer.
func TestHandleCompileProfile_InvalidSkillsJSON(t *testing.T) {
	tmpData := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpData)

	req := callToolRequest("compile_profile", map[string]any{"skills": "not-valid-json"})
	result := mcpserver.HandleCompileProfile(context.Background(), &req)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, text)
	}
	if _, ok := resp["error"]; !ok {
		t.Error("expected error key for invalid skills JSON")
	}
}

// TestHandleCompileProfile_ValidJSON verifies that valid JSON strings are parsed and
// produce a successful compilation.
func TestHandleCompileProfile_ValidJSON(t *testing.T) {
	tmpData := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpData)
	// Create the go-apply data dir so Save can write.
	if err := os.MkdirAll(filepath.Join(tmpData, "go-apply"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	req := callToolRequest("compile_profile", map[string]any{
		"skills":  `["Go","Kubernetes"]`,
		"stories": `[{"accomplishment":"did Go work","tags":["Go"]}]`,
	})
	result := mcpserver.HandleCompileProfile(context.Background(), &req)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, text)
	}
	assertStringKey(t, resp, "status", "compiled")

	// Kubernetes not covered by any story → should be orphaned.
	orphans, _ := resp["orphaned_skills"].([]interface{})
	if len(orphans) != 1 {
		t.Errorf("orphaned_skills = %v; want 1 (Kubernetes)", orphans)
	}
}
