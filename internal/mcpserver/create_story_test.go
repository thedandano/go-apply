package mcpserver_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// stubCreator is a test double for port.StoryCreatorService.
type stubCreator struct {
	out model.StoryOutput
	err error
}

var _ port.StoryCreatorService = (*stubCreator)(nil)

func (s *stubCreator) Create(_ context.Context, _ model.StoryInput) (model.StoryOutput, error) { //nolint:gocritic // hugeParam: interface signature
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

// TestHandleCreateStory_HappyPath verifies a successful create returns story_id and needs_compile.
func TestHandleCreateStory_HappyPath(t *testing.T) {
	creator := &stubCreator{out: model.StoryOutput{StoryID: "0"}}
	result := mcpserver.HandleCreateStoryWith(context.Background(), baseStoryArgs(), creator)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v\nraw: %s", err, text)
	}
	if resp["story_id"] != "0" {
		t.Errorf("story_id=%q; want \"0\"", resp["story_id"])
	}
	if resp["needs_compile"] != true {
		t.Errorf("needs_compile=%v; want true", resp["needs_compile"])
	}
}

// TestHandleCreateStory_CreatorError returns flat error JSON on creator failure.
func TestHandleCreateStory_CreatorError(t *testing.T) {
	creator := &stubCreator{err: model.ErrUnevidencedSkill}
	result := mcpserver.HandleCreateStoryWith(context.Background(), baseStoryArgs(), creator)
	text := extractText(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v\nraw: %s", err, text)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected 'error' key on creator failure, got: %s", text)
	}
}

// TestHandleCreateStory_MissingSkillArg verifies validation before calling creator.
func TestHandleCreateStory_MissingSkillArg(t *testing.T) {
	args := baseStoryArgs()
	delete(args, "skill")

	creator := &stubCreator{}
	result := mcpserver.HandleCreateStoryWith(context.Background(), args, creator)
	text := extractText(t, result)

	var resp map[string]interface{}
	_ = json.Unmarshal([]byte(text), &resp)
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected 'error' for missing skill arg, got: %s", text)
	}
}
