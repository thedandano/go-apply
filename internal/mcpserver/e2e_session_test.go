//go:build integration

package mcpserver

import (
	"testing"

	"github.com/thedandano/go-apply/internal/model"
)

// TestFullMCPSessionFlow tests the happy-path MCP session lifecycle from load_jd
// through finalize using the disk-backed SessionStore.
func TestFullMCPSessionFlow(t *testing.T) {
	dataDir := t.TempDir()

	store := NewSessionStore(dataDir)

	// Create a session and set initial state.
	sess := store.Create("raw jd text for full flow test")
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.State != stateLoaded {
		t.Fatalf("initial state = %v, want stateLoaded", sess.State)
	}

	// Step 1: Save in stateLoaded and reload — verify persistence.
	if err := store.Save(sess); err != nil {
		t.Fatalf("Save (stateLoaded): %v", err)
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load (stateLoaded): %v", err)
	}
	if loaded.State != stateLoaded {
		t.Errorf("loaded state = %v, want stateLoaded", loaded.State)
	}
	if loaded.ID != sess.ID {
		t.Errorf("loaded ID = %q, want %q", loaded.ID, sess.ID)
	}

	// Step 2: Transition to stateScored, save, reload — verify.
	loaded.State = stateScored
	if err := store.Save(loaded); err != nil {
		t.Fatalf("Save (stateScored): %v", err)
	}

	scored, err := store.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load (stateScored): %v", err)
	}
	if scored.State != stateScored {
		t.Errorf("scored state = %v, want stateScored", scored.State)
	}

	// Step 3: Transition to stateT1Applied, save, reload — verify.
	scored.State = stateT1Applied
	if err := store.Save(scored); err != nil {
		t.Fatalf("Save (stateT1Applied): %v", err)
	}

	t1, err := store.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load (stateT1Applied): %v", err)
	}
	if t1.State != stateT1Applied {
		t.Errorf("t1 state = %v, want stateT1Applied", t1.State)
	}

	// Step 4: Delete (simulating finalize cleanup) — verify gone.
	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Load(sess.ID)
	if err == nil {
		t.Error("expected error loading deleted session, got nil")
	}
}

// TestSessionPersistence simulates a process restart by creating two independent
// SessionStore instances pointing to the same data directory. Data written via
// store1 must be readable via store2 without any shared in-memory state.
func TestSessionPersistence(t *testing.T) {
	dataDir := t.TempDir()

	// ── store1: write phase ───────────────────────────────────────────────────

	store1 := NewSessionStore(dataDir)

	sess := store1.Create("golang kubernetes senior engineer")
	sess.JDText = "We are hiring a Senior Go Engineer with Kubernetes experience."
	sess.IsText = true
	sess.JD = model.JDData{
		Title:     "Senior Go Engineer",
		Company:   "Acme Corp",
		Required:  []string{"go", "kubernetes"},
		Preferred: []string{"docker", "terraform"},
		Seniority: model.SenioritySenior,
	}

	if err := store1.Save(sess); err != nil {
		t.Fatalf("store1 Save: %v", err)
	}
	sessionID := sess.ID

	// ── Simulate process restart: create a NEW store pointing to same dir ────
	// store1 is never used again after this point.
	store2 := NewSessionStore(dataDir)

	// ── store2: read phase ────────────────────────────────────────────────────

	restored, err := store2.Load(sessionID)
	if err != nil {
		t.Fatalf("store2 Load after restart: %v", err)
	}

	// Assert all fields survived the restart.
	if restored.ID != sessionID {
		t.Errorf("restored ID = %q, want %q", restored.ID, sessionID)
	}
	if restored.State != stateLoaded {
		t.Errorf("restored State = %v, want stateLoaded", restored.State)
	}
	if restored.JDText != sess.JDText {
		t.Errorf("restored JDText = %q, want %q", restored.JDText, sess.JDText)
	}
	if !restored.IsText {
		t.Error("restored IsText = false, want true")
	}
	if restored.JD.Title != "Senior Go Engineer" {
		t.Errorf("restored JD.Title = %q, want Senior Go Engineer", restored.JD.Title)
	}
	if restored.JD.Company != "Acme Corp" {
		t.Errorf("restored JD.Company = %q, want Acme Corp", restored.JD.Company)
	}
	if len(restored.JD.Required) != 2 {
		t.Errorf("restored JD.Required len = %d, want 2", len(restored.JD.Required))
	}

	// ── store2: continue the workflow after restart ───────────────────────────

	restored.State = stateScored
	if err := store2.Save(restored); err != nil {
		t.Fatalf("store2 Save (stateScored): %v", err)
	}

	final, err := store2.Load(sessionID)
	if err != nil {
		t.Fatalf("store2 Load (stateScored): %v", err)
	}
	if final.State != stateScored {
		t.Errorf("final state = %v, want stateScored", final.State)
	}
}
