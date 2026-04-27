package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/testutil/llmstub"
)

// writingOnboardStub is a stubOnboarder that also writes skills.md + accomplishments-0.md
// to the provided dataDir so recompilation has source files to process.
type writingOnboardStub struct {
	dataDir string
}

func (s *writingOnboardStub) Run(_ context.Context, _ model.OnboardInput) (model.OnboardResult, error) {
	if err := os.WriteFile(filepath.Join(s.dataDir, "skills.md"), []byte("Go\nKubernetes"), 0o600); err != nil {
		return model.OnboardResult{}, err
	}
	if err := os.WriteFile(filepath.Join(s.dataDir, "accomplishments-0.md"),
		[]byte("## Go — technical @ Backend Engineer\n**Situation:** s\n**Behavior:** b\n**Impact:** i"),
		0o600); err != nil {
		return model.OnboardResult{}, err
	}
	return model.OnboardResult{
		Stored:  []string{"resume:backend"},
		Summary: model.OnboardSummary{ResumesAdded: 1},
	}, nil
}

func newOnboardRequest(args map[string]interface{}) *mcp.CallToolRequest {
	req := &mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// TestHandleOnboardUserWith_TriggersRecompile verifies that HandleOnboardUserWith triggers
// recompilation and includes orphaned_skills in the response (US5 mutation wiring).
func TestHandleOnboardUserWith_TriggersRecompile(t *testing.T) {
	dir := t.TempDir()
	svc := &writingOnboardStub{dataDir: dir}
	stub := llmstub.New(map[string]string{}, 0, "")

	req := newOnboardRequest(map[string]interface{}{
		"resume_content": "my resume",
		"resume_label":   "backend",
	})

	result := mcpserver.HandleOnboardUserWith(context.Background(), req, svc, stub, nil, dir)
	text := textFrom(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v\nraw: %s", err, text)
	}

	// Base onboard fields must be present.
	if _, ok := resp["stored"]; !ok {
		t.Error("response missing 'stored' key")
	}
	if _, ok := resp["summary"]; !ok {
		t.Error("response missing 'summary' key")
	}

	// compile sub-object must be present and contain orphaned_skills.
	compile, ok := resp["compile"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing or wrong type 'compile' key; response: %s", text)
	}
	if _, ok := compile["orphaned_skills"]; !ok {
		t.Error("compile object missing 'orphaned_skills'")
	}
	// "Go" and "Kubernetes" have no stories evidencing them (LLM returns "[]") → both orphaned.
	orphans, _ := compile["orphaned_skills"].([]interface{})
	if len(orphans) == 0 {
		t.Error("orphaned_skills empty; want at least one (Go or Kubernetes not evidenced)")
	}
}

// TestHandleOnboardUserWith_NilLLM_SkipsCompile verifies that when llmClient is nil,
// the compile step is skipped and the response has no 'compile' key.
func TestHandleOnboardUserWith_NilLLM_SkipsCompile(t *testing.T) {
	dir := t.TempDir()
	svc := &writingOnboardStub{dataDir: dir}

	req := newOnboardRequest(map[string]interface{}{
		"resume_content": "my resume",
		"resume_label":   "backend",
	})

	result := mcpserver.HandleOnboardUserWith(context.Background(), req, svc, nil, nil, dir)
	text := textFrom(t, result)

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("parse response: %v\nraw: %s", err, text)
	}

	if _, ok := resp["compile"]; ok {
		t.Error("compile key present when llmClient=nil; want absent")
	}
	if _, ok := resp["stored"]; !ok {
		t.Error("response missing 'stored' key")
	}
}
