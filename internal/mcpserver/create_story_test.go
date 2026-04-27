package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/testutil/llmstub"
)

// stubCreator is a test double for port.StoryCreatorService.
type stubCreator struct {
	out model.StoryOutput
	err error
}

func (s *stubCreator) Create(_ context.Context, _ model.StoryInput) (model.StoryOutput, error) { //nolint:gocritic // hugeParam: test stub, interface signature fixed
	return s.out, s.err
}

func baseStoryArgs() map[string]interface{} {
	return map[string]interface{}{
		"skill":      "Go",
		"story_type": "technical",
		"job_title":  "Backend Engineer",
		"situation":  "Team needed faster API",
		"behavior":   "I rewrote the handler",
		"impact":     "Latency dropped 40%",
	}
}

// TestHandleCreateStory_HappyPath verifies a successful end-to-end flow.
func TestHandleCreateStory_HappyPath(t *testing.T) {
	dir := t.TempDir()
	// Write skills.md so recompilation can run.
	if err := os.WriteFile(filepath.Join(dir, "skills.md"), []byte("Go\nKubernetes"), 0o600); err != nil {
		t.Fatalf("write skills.md: %v", err)
	}
	// Write the story file that stubCreator will claim to have created — recompile needs it on disk.
	storyContent := "## Go — technical @ Backend Engineer\n**Situation:** s\n**Behavior:** b\n**Impact:** i"
	if err := os.WriteFile(filepath.Join(dir, "accomplishments-0.md"), []byte(storyContent), 0o600); err != nil {
		t.Fatalf("write accomplishments-0.md: %v", err)
	}

	creator := &stubCreator{out: model.StoryOutput{SourceFile: "accomplishments-0.md"}}
	stub := llmstub.New(map[string]string{}, 0, "")

	result := mcpserver.HandleCreateStoryWith(
		context.Background(), dir,
		baseStoryArgs(), creator, stub,
	)
	text := textFrom(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v\nraw: %s", err, text)
	}

	for _, key := range []string{"story_id", "source_file", "skills_tagged", "compiled_at", "remaining_orphans"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("key %q missing from response", key)
		}
	}
	if resp["source_file"] != "accomplishments-0.md" {
		t.Errorf("source_file=%q; want accomplishments-0.md", resp["source_file"])
	}
	if storyID, _ := resp["story_id"].(string); storyID == "" {
		t.Errorf("story_id is empty; compiled profile should have story for accomplishments-0.md")
	}
}

// TestHandleCreateStory_CreatorError returns flat error JSON on creator failure.
func TestHandleCreateStory_CreatorError(t *testing.T) {
	dir := t.TempDir()
	creator := &stubCreator{err: model.ErrUnevidencedSkill}
	stub := llmstub.New(map[string]string{}, 0, "")

	result := mcpserver.HandleCreateStoryWith(
		context.Background(), dir,
		baseStoryArgs(), creator, stub,
	)
	text := textFrom(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v\nraw: %s", err, text)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected 'error' key on creator failure, got: %s", text)
	}
}

// TestHandleCreateStory_RecompileAfterCreate verifies the profile reflects the new story.
func TestHandleCreateStory_RecompileAfterCreate(t *testing.T) {
	dir := t.TempDir()
	// Write a story file so recompilation has something to process.
	storyContent := "## Go — technical @ Backend Engineer\n**Situation:** s\n**Behavior:** b\n**Impact:** i"
	if err := os.WriteFile(filepath.Join(dir, "accomplishments-0.md"), []byte(storyContent), 0o600); err != nil {
		t.Fatalf("write story file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills.md"), []byte("Go"), 0o600); err != nil {
		t.Fatalf("write skills.md: %v", err)
	}

	// Stub creator returns the file we just wrote.
	creator := &stubCreator{out: model.StoryOutput{SourceFile: "accomplishments-0.md"}}
	// Stub LLM returns ["Go"] for any story.
	stub := llmstub.New(map[string]string{}, 0, "")

	result := mcpserver.HandleCreateStoryWith(
		context.Background(), dir,
		baseStoryArgs(), creator, stub,
	)
	text := textFrom(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v\nraw: %s", err, text)
	}

	// story_id must reference a compiled story.
	storyID, _ := resp["story_id"].(string)
	if storyID == "" {
		t.Errorf("story_id empty; response: %s", text)
	}
}

// TestHandleCreateStory_MissingSkillArg verifies validation before calling creator.
func TestHandleCreateStory_MissingSkillArg(t *testing.T) {
	dir := t.TempDir()
	args := baseStoryArgs()
	delete(args, "skill")

	creator := &stubCreator{}
	stub := llmstub.New(map[string]string{}, 0, "")

	result := mcpserver.HandleCreateStoryWith(
		context.Background(), dir,
		args, creator, stub,
	)
	text := textFrom(t, result)

	var resp map[string]interface{}
	_ = json.Unmarshal([]byte(text), &resp)
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected 'error' for missing skill arg, got: %s", text)
	}
}
