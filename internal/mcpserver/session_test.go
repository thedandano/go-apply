package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionState_String(t *testing.T) {
	cases := []struct {
		s    sessionState
		want string
	}{
		{stateLoaded, "loaded"},
		{stateScored, "scored"},
		{stateT1Applied, "t1_applied"},
		{stateT2Applied, "t2_applied"},
		{stateFinalized, "finalized"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("state %d String() = %q, want %q", c.s, got, c.want)
		}
	}
}

// ── disk-backed SessionStore tests (T001) ─────────────────────────────────────
// These tests target the NEW disk-backed API (NewSessionStore(dataDir string)).
// They are intentionally RED until the implementation is written.

func TestSessionStore_SaveAndLoad(t *testing.T) {
	dataDir := t.TempDir()
	store := NewSessionStore(dataDir)

	now := time.Now().UTC().Truncate(time.Second)
	sess := &Session{
		ID:        newSessionID(),
		State:     stateScored,
		JDText:    "looking for a Go engineer",
		CreatedAt: now,
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load(sess.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ID != sess.ID {
		t.Errorf("ID: got %q, want %q", got.ID, sess.ID)
	}
	if got.State != sess.State {
		t.Errorf("State: got %v, want %v", got.State, sess.State)
	}
	if got.JDText != sess.JDText {
		t.Errorf("JDText: got %q, want %q", got.JDText, sess.JDText)
	}
	if !got.CreatedAt.Equal(sess.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, sess.CreatedAt)
	}
}

func TestSessionStore_Load_NotFound(t *testing.T) {
	dataDir := t.TempDir()
	store := NewSessionStore(dataDir)

	_, err := store.Load("nonexistent-id")
	if err == nil {
		t.Fatal("expected error when loading nonexistent session, got nil")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	dataDir := t.TempDir()
	store := NewSessionStore(dataDir)

	sess := &Session{
		ID:        newSessionID(),
		State:     stateLoaded,
		JDText:    "delete me",
		CreatedAt: time.Now().UTC(),
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := store.Delete(sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := store.Load(sess.ID)
	if err == nil {
		t.Fatal("expected error after Delete, got nil — session still loadable")
	}
}

func TestSessionStore_SweepExpired(t *testing.T) {
	dataDir := t.TempDir()
	store := NewSessionStore(dataDir)

	// Create 3 sessions that will be backdated to appear stale.
	staleSessions := make([]*Session, 3)
	for i := range staleSessions {
		staleSessions[i] = &Session{
			ID:        newSessionID(),
			State:     stateLoaded,
			JDText:    fmt.Sprintf("stale jd %d", i),
			CreatedAt: time.Now().UTC(),
		}
		if err := store.Save(staleSessions[i]); err != nil {
			t.Fatalf("Save stale[%d]: %v", i, err)
		}
		// Backdate the file mtime to 2 hours ago so it falls outside the 1-hour TTL.
		filePath := filepath.Join(dataDir, "sessions", staleSessions[i].ID+".json")
		oldTime := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(filePath, oldTime, oldTime); err != nil {
			t.Fatalf("Chtimes stale[%d]: %v", i, err)
		}
	}

	// Create one fresh session (mtime is just now — must survive the sweep).
	freshSess := &Session{
		ID:        newSessionID(),
		State:     stateLoaded,
		JDText:    "fresh jd",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.Save(freshSess); err != nil {
		t.Fatalf("Save fresh: %v", err)
	}

	// Sweep with a 1-hour TTL — anything older than 1 hour should be deleted.
	store.SweepExpired(1 * time.Hour)

	// The 3 stale sessions must be gone.
	for i, stale := range staleSessions {
		if _, err := store.Load(stale.ID); err == nil {
			t.Errorf("stale[%d] still exists after SweepExpired", i)
		}
	}

	// The fresh session must still be present.
	if _, err := store.Load(freshSess.ID); err != nil {
		t.Errorf("fresh session was incorrectly swept: %v", err)
	}
}

func TestSessionStore_Save_AtomicWrite(t *testing.T) {
	dataDir := t.TempDir()
	store := NewSessionStore(dataDir)

	sess := &Session{
		ID:        newSessionID(),
		State:     stateLoaded,
		JDText:    "atomic write test",
		CreatedAt: time.Now().UTC(),
	}

	if err := store.Save(sess); err != nil {
		t.Fatalf("Save: %v", err)
	}

	expectedPath := filepath.Join(dataDir, "sessions", sess.ID+".json")
	info, err := os.Stat(expectedPath)
	if err != nil {
		t.Fatalf("session file not found at expected path %q: %v", expectedPath, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file permissions = %o, want 0600", info.Mode().Perm())
	}
}
