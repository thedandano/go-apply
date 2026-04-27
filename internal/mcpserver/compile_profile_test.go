package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/testutil/llmstub"
)

func textFrom(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("CallToolResult has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is not TextContent: %T", result.Content[0])
	}
	return tc.Text
}

// TestHandleCompileProfile_HappyPath verifies compilation runs and returns correct schema.
func TestHandleCompileProfile_HappyPath(t *testing.T) {
	dir := t.TempDir()
	writeCompileTestFile(t, filepath.Join(dir, "skills.md"), "Go\nKubernetes")
	writeCompileTestFile(t, filepath.Join(dir, "accomplishments-0.md"), "## Go — technical @ Eng\n**Situation:** s\n**Behavior:** b\n**Impact:** i")

	stub := llmstub.New(map[string]string{}, 0, "")
	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, stub)
	text := textFrom(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v\nraw: %s", err, text)
	}

	assertStringKey(t, resp, "schema_version", "1")
	assertStringKey(t, resp, "status", "compiled")
	for _, key := range []string{"compiled_at", "stories", "orphaned_skills", "partial_tagging_failure"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("key %q missing from response", key)
		}
	}
}

// TestHandleCompileProfile_AlreadyUpToDate — arrays must still be populated.
func TestHandleCompileProfile_AlreadyUpToDate(t *testing.T) {
	dir := t.TempDir()

	accPath := filepath.Join(dir, "accomplishments-0.md")
	writeCompileTestFile(t, accPath, "## Go — technical @ Eng\n**Situation:** s\n**Behavior:** b\n**Impact:** i")
	old := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(accPath, old, old)

	profile := model.CompiledProfile{
		SchemaVersion:  "1",
		CompiledAt:     time.Now().UTC(),
		Stories:        []model.Story{{ID: "story-001", SourceFile: "accomplishments-0.md", Text: "t", Skills: []string{"Go"}, Format: "SBI", Type: model.StoryTypeTechnical, JobTitle: "Eng"}},
		OrphanedSkills: []model.OrphanedSkill{{Skill: "ArgoCD", Deferred: false}},
	}
	writeProfileFile(t, dir, profile)

	stub := llmstub.New(map[string]string{}, 0, "")
	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, stub)
	text := textFrom(t, result)

	var resp map[string]interface{}
	_ = json.Unmarshal([]byte(text), &resp)

	assertStringKey(t, resp, "status", "already_up_to_date")

	stories, _ := resp["stories"].([]interface{})
	if len(stories) == 0 {
		t.Error("stories[] empty on already_up_to_date; want populated from existing profile")
	}
	orphans, _ := resp["orphaned_skills"].([]interface{})
	if len(orphans) == 0 {
		t.Error("orphaned_skills[] empty on already_up_to_date; want populated from existing profile")
	}
}

// TestHandleCompileProfile_FirstRun — no profile must produce status=compiled, not already_up_to_date.
func TestHandleCompileProfile_FirstRun(t *testing.T) {
	dir := t.TempDir()
	writeCompileTestFile(t, filepath.Join(dir, "skills.md"), "Go")
	writeCompileTestFile(t, filepath.Join(dir, "accomplishments-0.md"), "## Go — technical @ Eng\n**Situation:** s\n**Behavior:** b\n**Impact:** i")

	stub := llmstub.New(map[string]string{}, 0, "")
	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, stub)
	text := textFrom(t, result)

	var resp map[string]interface{}
	_ = json.Unmarshal([]byte(text), &resp)

	if resp["status"] == "already_up_to_date" {
		t.Error("status=already_up_to_date on first run; want compiled")
	}
	assertStringKey(t, resp, "status", "compiled")
}

// TestHandleCompileProfile_PartialTaggingFailure verifies partial_tagging_failure surfaced.
func TestHandleCompileProfile_PartialTaggingFailure(t *testing.T) {
	dir := t.TempDir()
	writeCompileTestFile(t, filepath.Join(dir, "skills.md"), "Go")
	writeCompileTestFile(t, filepath.Join(dir, "accomplishments-0.md"), "## story")

	stub := llmstub.New(map[string]string{}, 1, "llm down")
	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, stub)
	text := textFrom(t, result)

	var resp map[string]interface{}
	_ = json.Unmarshal([]byte(text), &resp)

	if resp["partial_tagging_failure"] != true {
		t.Errorf("partial_tagging_failure=%v; want true", resp["partial_tagging_failure"])
	}
}

// TestHandleCompileProfile_EmptyDataDir — must not panic on missing sources.
func TestHandleCompileProfile_EmptyDataDir(t *testing.T) {
	dir := t.TempDir()
	stub := llmstub.New(map[string]string{}, 0, "")
	result := mcpserver.HandleCompileProfileWith(context.Background(), dir, stub)
	if result == nil {
		t.Fatal("nil result on empty dataDir")
	}
	text := textFrom(t, result)
	if text == "" {
		t.Error("empty text response")
	}
}

// helpers

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

func writeCompileTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeProfileFile(t *testing.T, dir string, p model.CompiledProfile) { //nolint:gocritic // hugeParam: test helper
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile-compiled.json"), data, 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}
