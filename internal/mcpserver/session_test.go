package mcpserver

import (
	"fmt"
	"testing"
)

func TestSessionStore_CreateAndGet(t *testing.T) {
	store := NewSessionStore()
	sess := store.Create("raw jd text")

	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.State != stateLoaded {
		t.Errorf("initial state = %v, want loaded", sess.State)
	}
	if sess.JDText != "raw jd text" {
		t.Errorf("JDText = %q, want %q", sess.JDText, "raw jd text")
	}

	got := store.Get(sess.ID)
	if got == nil {
		t.Fatal("Get returned nil for existing session")
	}
	if got.ID != sess.ID {
		t.Errorf("got ID %q, want %q", got.ID, sess.ID)
	}
}

func TestSessionStore_GetMissing_ReturnsNil(t *testing.T) {
	store := NewSessionStore()
	got := store.Get("nonexistent")
	if got != nil {
		t.Errorf("expected nil for missing session, got %+v", got)
	}
}

func TestSessionStore_LRUEviction(t *testing.T) {
	store := NewSessionStore()

	// Fill to capacity.
	first := store.Create("first")
	for i := 1; i < sessionStoreCap; i++ {
		store.Create(fmt.Sprintf("jd %d", i))
	}

	// First session should still be present.
	if store.Get(first.ID) == nil {
		t.Fatal("first session evicted before capacity exceeded")
	}

	// Touch first to make it recently used.
	store.Get(first.ID)

	// Add one more to trigger eviction (second created becomes oldest).
	store.Create("overflow")

	// First session (recently touched) must still be present.
	if store.Get(first.ID) == nil {
		t.Error("recently-touched session was incorrectly evicted")
	}
}

func TestSessionStore_IDsAreUnique(t *testing.T) {
	store := NewSessionStore()
	ids := make(map[string]struct{}, 50)
	for range 50 {
		s := store.Create("jd")
		if _, dup := ids[s.ID]; dup {
			t.Fatalf("duplicate session ID: %q", s.ID)
		}
		ids[s.ID] = struct{}{}
	}
}

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
