package sessionstore_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/thedandano/go-apply/internal/sessionstore"
)

func TestMemoryStore_CreateAndGet(t *testing.T) {
	store := sessionstore.NewMemoryStore()
	ctx := context.Background()

	sess, err := store.Create(ctx, "raw jd text")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if sess.State != sessionstore.StateLoaded {
		t.Errorf("initial state = %q, want %q", sess.State, sessionstore.StateLoaded)
	}
	if sess.JDText != "raw jd text" {
		t.Errorf("JDText = %q, want %q", sess.JDText, "raw jd text")
	}
	if sess.CreatedAt.IsZero() {
		t.Error("CreatedAt must not be zero")
	}

	got, ok, err := store.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get returned not-found for existing session")
	}
	if got.ID != sess.ID {
		t.Errorf("got ID %q, want %q", got.ID, sess.ID)
	}
}

func TestMemoryStore_GetMissing_ReturnsNotFound(t *testing.T) {
	store := sessionstore.NewMemoryStore()
	got, ok, err := store.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok || got != nil {
		t.Errorf("expected not-found, got %+v ok=%v", got, ok)
	}
}

func TestMemoryStore_Update(t *testing.T) {
	store := sessionstore.NewMemoryStore()
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd text")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sess.State = sessionstore.StateScored
	if err := store.Update(ctx, sess); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, ok, err := store.Get(ctx, sess.ID)
	if err != nil || !ok {
		t.Fatalf("Get after Update: err=%v ok=%v", err, ok)
	}
	if got.State != sessionstore.StateScored {
		t.Errorf("state after Update = %q, want %q", got.State, sessionstore.StateScored)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := sessionstore.NewMemoryStore()
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd text")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, ok, err := store.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get after Delete: %v", err)
	}
	if ok {
		t.Error("session still exists after Delete")
	}
}

func TestMemoryStore_LRUEviction(t *testing.T) {
	store := sessionstore.NewMemoryStore()
	ctx := context.Background()

	// Fill to capacity.
	first, err := store.Create(ctx, "first")
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	for i := 1; i < 100; i++ {
		if _, err := store.Create(ctx, fmt.Sprintf("jd %d", i)); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// First session should still be present.
	if _, ok, _ := store.Get(ctx, first.ID); !ok {
		t.Fatal("first session evicted before capacity exceeded")
	}

	// Touch first to make it recently used.
	store.Get(ctx, first.ID) //nolint:errcheck

	// Add one more to trigger eviction (second created becomes oldest).
	if _, err := store.Create(ctx, "overflow"); err != nil {
		t.Fatalf("Create overflow: %v", err)
	}

	// First session (recently touched) must still be present.
	if _, ok, _ := store.Get(ctx, first.ID); !ok {
		t.Error("recently-touched session was incorrectly evicted")
	}
}

func TestMemoryStore_IDsAreUnique(t *testing.T) {
	store := sessionstore.NewMemoryStore()
	ctx := context.Background()
	ids := make(map[string]struct{}, 50)
	for range 50 {
		s, err := store.Create(ctx, "jd")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if _, dup := ids[s.ID]; dup {
			t.Fatalf("duplicate session ID: %q", s.ID)
		}
		ids[s.ID] = struct{}{}
	}
}
