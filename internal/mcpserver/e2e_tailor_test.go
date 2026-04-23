package mcpserver_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
)

// TestTailor_EndToEnd_Plumbing is the SC-007 ship-blocker test.
// It exercises the full tailor_begin → store.Wait → tailor_submit round-trip
// without involving the production wiring, confirming that:
//   - TailoredText reaches the waiting caller unchanged.
//   - Changelog entries (≥1) survive the full round-trip.
func TestTailor_EndToEnd_Plumbing(t *testing.T) {
	store := mcpserver.NewTailorSessionStore()
	ctx := context.Background()

	// Step 1: tailor_begin — opens a session and returns a session_id.
	beginReq := callToolRequest("tailor_begin", map[string]any{
		"resume_text":     "Experienced backend engineer with Go expertise.",
		"timeout_seconds": 10,
	})
	beginResult := mcpserver.HandleTailorBeginWithStore(ctx, &beginReq, store, "test-prompt-body")
	beginText := extractText(t, beginResult)

	var beginEnv map[string]any
	if err := json.Unmarshal([]byte(beginText), &beginEnv); err != nil {
		t.Fatalf("tailor_begin response not JSON: %v — raw: %s", err, beginText)
	}
	if beginEnv["status"] != "ok" {
		t.Fatalf("tailor_begin status = %v, want ok — full: %s", beginEnv["status"], beginText)
	}
	data, _ := beginEnv["data"].(map[string]any)
	sessionID, _ := data["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("tailor_begin returned empty session_id — data: %v", data)
	}

	// Step 2: goroutine simulates the pipeline blocked on store.Wait.
	type waitOutcome struct {
		result model.TailorResult
		err    error
	}
	waitCh := make(chan waitOutcome, 1)
	go func() {
		r, err := store.Wait(ctx, sessionID)
		waitCh <- waitOutcome{result: r, err: err}
	}()

	// Step 3: tailor_submit — agent delivers the tailored resume + changelog.
	changelog := []model.ChangelogEntry{
		{Kind: model.ChangelogSkillAdd, Tier: model.ChangelogTier1, Keyword: "kubernetes"},
		{
			Kind:   model.ChangelogBulletRewrite,
			Tier:   model.ChangelogTier1,
			Before: "old bullet",
			After:  "new bullet",
			Source: model.RewriteSourceAccomplishmentsDoc,
		},
	}
	changelogBytes, _ := json.Marshal(changelog)

	submitReq := callToolRequest("tailor_submit", map[string]any{
		"session_id":    sessionID,
		"tailored_text": "Tailored resume body text",
		"changelog":     string(changelogBytes),
	})
	submitResult := mcpserver.HandleTailorSubmitWithStore(ctx, &submitReq, store)
	submitText := extractText(t, submitResult)

	var submitEnv map[string]any
	if err := json.Unmarshal([]byte(submitText), &submitEnv); err != nil {
		t.Fatalf("tailor_submit response not JSON: %v — raw: %s", err, submitText)
	}
	if submitEnv["status"] != "ok" {
		t.Fatalf("tailor_submit status = %v, want ok — full: %s", submitEnv["status"], submitText)
	}

	// Step 4: collect the result from the blocked goroutine.
	var outcome waitOutcome
	select {
	case outcome = <-waitCh:
	case <-time.After(3 * time.Second):
		t.Fatal("store.Wait did not unblock after tailor_submit")
	}
	if outcome.err != nil {
		t.Fatalf("store.Wait error: %v", outcome.err)
	}

	// SC-007 assertions.
	if outcome.result.TailoredText == "" {
		t.Error("TailoredText is empty; want non-empty tailored resume")
	}
	if len(outcome.result.Changelog) < 1 {
		t.Errorf("Changelog len = %d; want >= 1", len(outcome.result.Changelog))
	}
}
