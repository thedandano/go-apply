//go:build !windows

package sessionstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDiskStore_Create_ExistingFile_Fails verifies that writeSession(isCreate=true)
// returns ErrSessionExists when the target file already exists on disk.
// This exercises the O_EXCL flag added to the Create path.
func TestDiskStore_Create_ExistingFile_Fails(t *testing.T) {
	dir := t.TempDir()
	store := &DiskStore{dir: dir}

	sess := &Session{
		ID:        "aaabbbcccdddeeefffggg000111222",
		State:     StateLoaded,
		JDText:    "jd text",
		CreatedAt: time.Now().UTC(),
	}

	// Pre-create the file so that a subsequent Create write to the same path hits O_EXCL.
	path := filepath.Join(dir, sess.ID+".json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("pre-create file: %v", err)
	}

	// writeSession with isCreate=true must fail with ErrSessionExists.
	err := store.writeSession(context.Background(), sess, true)
	if err == nil {
		t.Fatal("expected error when file already exists, got nil")
	}
	if !errors.Is(err, ErrSessionExists) {
		t.Errorf("expected errors.Is(err, ErrSessionExists); got: %v", err)
	}
}
