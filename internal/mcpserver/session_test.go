package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/sessionstore"
)

// whiteBoxCapturingRepo captures the last record passed to Put for white-box handler tests.
type whiteBoxCapturingRepo struct {
	last *model.ApplicationRecord
}

var _ port.ApplicationRepository = (*whiteBoxCapturingRepo)(nil)

func (r *whiteBoxCapturingRepo) Get(_ string) (*model.ApplicationRecord, bool, error) {
	return nil, false, nil
}
func (r *whiteBoxCapturingRepo) Put(rec *model.ApplicationRecord) error {
	clone := *rec
	r.last = &clone
	return nil
}
func (r *whiteBoxCapturingRepo) Update(_ *model.ApplicationRecord) error   { return nil }
func (r *whiteBoxCapturingRepo) List() ([]*model.ApplicationRecord, error) { return nil, nil }

// whiteBoxCallToolRequest builds a bare mcp.CallToolRequest for white-box tests.
func whiteBoxCallToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: name, Arguments: args},
	}
}

// TestHandleFinalize_TailoredSession_TailorResultAndChangelogPersisted verifies that when a
// session carries TailoredText and a non-empty Changelog, HandleFinalizeWithConfig constructs
// and persists a TailorResult with ResumeLabel, NewScore, and the full Changelog.
func TestHandleFinalize_TailoredSession_TailorResultAndChangelogPersisted(t *testing.T) {
	changelog := []model.ChangelogEntry{
		{Action: "added", Target: "skill", Keyword: "kubernetes", Reason: "required by JD"},
		{Action: "rewrote", Target: "bullet", Keyword: "go"},
		{Action: "skipped", Target: "summary", Keyword: "docker"},
	}

	// Use an isolated MemoryStore for this test rather than the package-level store.
	testStore := sessionstore.NewMemoryStore()
	ctx := context.Background()

	sess, err := testStore.Create(ctx, "raw jd for tailored test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sess.URL = "https://example.com/job/tailored-whitebox"
	sess.State = sessionstore.StateTailored
	sess.TailoredText = "full tailored resume body"
	sess.Changelog = changelog
	sess.ScoreResult = pipeline.ScoreResumeResult{
		BestLabel: "main",
		BestScore: 85.0,
		Scores: map[string]model.ScoreResult{
			"main": {ResumeLabel: "main"},
		},
	}
	if err := testStore.Update(ctx, sess); err != nil {
		t.Fatalf("Update: %v", err)
	}

	capturing := &whiteBoxCapturingRepo{}
	deps := pipeline.ApplyConfig{
		Fetcher:  nil,
		Scorer:   nil,
		Resumes:  nil,
		Loader:   nil,
		AppRepo:  capturing,
		Defaults: &config.AppDefaults{},
	}

	req := whiteBoxCallToolRequest("finalize", map[string]any{"session_id": sess.ID})
	result := HandleFinalizeWithConfig(ctx, &req, &deps, testStore)

	if len(result.Content) == 0 {
		t.Fatal("HandleFinalizeWithConfig returned no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is not TextContent: %T", result.Content[0])
	}
	text := tc.Text

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("finalize response not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Fatalf("finalize status = %v, want ok — full: %s", env["status"], text)
	}

	// The capturing repo must have received a record.
	if capturing.last == nil {
		t.Fatal("AppRepo.Put was not called; expected a persisted record for URL session")
	}

	// TailorResult must be populated.
	if capturing.last.TailorResult == nil {
		t.Fatal("persisted TailorResult is nil; expected it to be set for a tailored session")
	}
	tr := capturing.last.TailorResult

	if tr.ResumeLabel != "main" {
		t.Errorf("TailorResult.ResumeLabel = %q, want %q", tr.ResumeLabel, "main")
	}

	// Changelog must round-trip losslessly.
	if len(tr.Changelog) != len(changelog) {
		t.Fatalf("TailorResult.Changelog length = %d, want %d", len(tr.Changelog), len(changelog))
	}
	for i, want := range changelog {
		got := tr.Changelog[i]
		if got.Action != want.Action || got.Target != want.Target || got.Keyword != want.Keyword || got.Reason != want.Reason {
			t.Errorf("Changelog[%d] = %+v, want %+v", i, got, want)
		}
	}

	// Marshal the persisted record and verify tailor_result is present and tailored_text is absent.
	persisted, err := json.Marshal(capturing.last)
	if err != nil {
		t.Fatalf("marshal persisted record: %v", err)
	}
	if !strings.Contains(string(persisted), `"tailor_result"`) {
		t.Errorf("persisted JSON missing tailor_result; got: %s", persisted)
	}
	if strings.Contains(string(persisted), `"tailored_text"`) {
		t.Errorf("persisted JSON must not contain tailored_text (redacted); got: %s", persisted)
	}
}
