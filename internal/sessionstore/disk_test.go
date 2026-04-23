package sessionstore_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"testing"

	"github.com/thedandano/go-apply/internal/sessionstore"
)

// sessionIDRegex validates that IDs match the spec regex ^[a-z0-9]{26,64}$.
var sessionIDRegex = regexp.MustCompile(`^[a-z0-9]{26,64}$`)

func newTestDiskStore(t *testing.T) (*sessionstore.DiskStore, string) {
	t.Helper()
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	store, err := sessionstore.NewDiskStore(sessDir)
	if err != nil {
		t.Fatalf("NewDiskStore: %v", err)
	}
	return store, sessDir
}

func TestDiskStore_CreateAndGet(t *testing.T) {
	store, _ := newTestDiskStore(t)
	ctx := context.Background()

	sess, err := store.Create(ctx, "raw jd text")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !sessionIDRegex.MatchString(sess.ID) {
		t.Errorf("session ID %q does not match regex", sess.ID)
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
	if got.JDText != sess.JDText {
		t.Errorf("JDText round-trip: got %q, want %q", got.JDText, sess.JDText)
	}
}

func TestDiskStore_GetMissing_ReturnsNotFound(t *testing.T) {
	store, _ := newTestDiskStore(t)
	got, ok, err := store.Get(context.Background(), "deadbeefdeadbeefdeadbeefdeadbeef")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok || got != nil {
		t.Errorf("expected not-found, got %+v ok=%v", got, ok)
	}
}

func TestDiskStore_FilePermissions(t *testing.T) {
	store, sessDir := newTestDiskStore(t)
	ctx := context.Background()

	// Check directory permissions.
	info, err := os.Stat(sessDir)
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fs.FileMode(0o700) {
		t.Errorf("dir perm = %04o, want 0700", perm)
	}

	// Create a session and check file permissions.
	sess, err := store.Create(ctx, "jd text")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	path := filepath.Join(sessDir, sess.ID+".json")
	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("Stat file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fs.FileMode(0o600) {
		t.Errorf("file perm = %04o, want 0600", perm)
	}
}

// TestDiskStore_FilePermissions_ParanoidUmask verifies that the session file is
// always created with mode 0o600, even when a very restrictive umask is active.
// A umask of 0o177 would yield mode 0o400 (read-only) if OpenFile's perm argument
// were the sole mode-setter; the explicit Chmod in writeSession overrides it.
//
// The temp directory is created before the umask is narrowed because t.TempDir()
// itself uses MkdirAll which is also subject to the umask.
func TestDiskStore_FilePermissions_ParanoidUmask(t *testing.T) {
	// Create the temp base dir before narrowing the umask.
	baseDir := t.TempDir()
	sessDir := filepath.Join(baseDir, "sessions")

	// Now set the paranoid umask — only file/dir creation inside writeSession
	// will be affected after this point.
	old := syscall.Umask(0o177)
	t.Cleanup(func() { syscall.Umask(old) })

	store, err := sessionstore.NewDiskStore(sessDir)
	if err != nil {
		t.Fatalf("NewDiskStore under paranoid umask: %v", err)
	}

	sess, err := store.Create(context.Background(), "jd text umask test")
	if err != nil {
		t.Fatalf("Create under paranoid umask: %v", err)
	}

	path := filepath.Join(sessDir, sess.ID+".json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fs.FileMode(0o600) {
		t.Errorf("file perm under umask 0o177 = %04o, want 0600 (Chmod must override umask)", perm)
	}
}

func TestDiskStore_ChmodOnReopen(t *testing.T) {
	store, sessDir := newTestDiskStore(t)
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd text")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	path := filepath.Join(sessDir, sess.ID+".json")

	// Manually widen the file permission to simulate a bad umask scenario.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod to widen: %v", err)
	}

	// Update must restore restrictive permissions.
	sess.State = sessionstore.StateScored
	if err := store.Update(ctx, sess); err != nil {
		t.Fatalf("Update: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat after update: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fs.FileMode(0o600) {
		t.Errorf("file perm after update = %04o, want 0600", perm)
	}
}

func TestDiskStore_Update(t *testing.T) {
	store, _ := newTestDiskStore(t)
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

func TestDiskStore_Delete(t *testing.T) {
	store, _ := newTestDiskStore(t)
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

func TestDiskStore_DeleteIdempotent(t *testing.T) {
	store, _ := newTestDiskStore(t)
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd text")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("first Delete: %v", err)
	}
	// Second delete on nonexistent file must not return an error.
	if err := store.Delete(ctx, sess.ID); err != nil {
		t.Fatalf("second Delete (idempotent): %v", err)
	}
}

func TestDiskStore_JSONRoundTrip(t *testing.T) {
	store, _ := newTestDiskStore(t)
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd text for round-trip")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sess.URL = "https://example.com/job"
	sess.IsText = false
	sess.State = sessionstore.StateScored
	if err := store.Update(ctx, sess); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _, err := store.Get(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Verify all fields round-trip.
	if got.ID != sess.ID {
		t.Errorf("ID: got %q want %q", got.ID, sess.ID)
	}
	if got.URL != sess.URL {
		t.Errorf("URL: got %q want %q", got.URL, sess.URL)
	}
	if got.IsText != sess.IsText {
		t.Errorf("IsText: got %v want %v", got.IsText, sess.IsText)
	}
	if got.JDText != sess.JDText {
		t.Errorf("JDText: got %q want %q", got.JDText, sess.JDText)
	}
	if got.State != sess.State {
		t.Errorf("State: got %q want %q", got.State, sess.State)
	}
	if !got.CreatedAt.Equal(sess.CreatedAt) {
		t.Errorf("CreatedAt: got %v want %v", got.CreatedAt, sess.CreatedAt)
	}
}

// TestDiskStore_ErrSessionLocked verifies that Update returns ErrSessionLocked when
// the session file is held by an exclusive flock on a separate open-file description.
// On Darwin (and Linux), flock is per open-file-description, so two os.OpenFile calls
// from the same process can reliably contend.
// The test also asserts that slog.WarnContext emits a structured log record with the
// expected message, level, session_id, pid, and path attributes.
func TestDiskStore_ErrSessionLocked(t *testing.T) {
	store, sessDir := newTestDiskStore(t)
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd for lock test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	path := filepath.Join(sessDir, sess.ID+".json")

	// Capture slog output so we can assert the warn record.
	var logBuf bytes.Buffer
	origDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(origDefault) })

	// Open a separate file description and acquire an exclusive non-blocking flock.
	// This simulates another process (or concurrent writer) holding the lock.
	holder, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open holder fd: %v", err)
	}
	defer holder.Close() //nolint:errcheck

	if lockErr := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); lockErr != nil {
		t.Skipf("flock unavailable on this platform/fs: %v", lockErr)
	}

	// Now try to update — store opens a NEW file description and should get EWOULDBLOCK.
	clone := *sess
	clone.State = sessionstore.StateScored
	updateErr := store.Update(ctx, &clone)

	// Release the lock so the file isn't left locked after the test.
	_ = syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)

	if updateErr == nil {
		t.Fatal("expected ErrSessionLocked, got nil (same-process flock may not contend on this platform)")
	}
	if !errors.Is(updateErr, sessionstore.ErrSessionLocked) {
		t.Errorf("expected errors.Is(err, ErrSessionLocked); got: %v", updateErr)
	}

	// Parse slog JSON lines and find the warn record.
	found := false
	for _, line := range bytes.Split(bytes.TrimSpace(logBuf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec["msg"] != "session locked by concurrent process" {
			continue
		}
		found = true
		// Assert level == WARN (slog JSON encodes level as "WARN").
		if rec["level"] != "WARN" {
			t.Errorf("log record level = %q, want %q", rec["level"], "WARN")
		}
		// Assert session_id attribute.
		if rec["session_id"] != sess.ID {
			t.Errorf("log record session_id = %q, want %q", rec["session_id"], sess.ID)
		}
		// Assert pid attribute equals current process pid (encoded as float64 in JSON).
		wantPID := float64(os.Getpid())
		if rec["pid"] != wantPID {
			t.Errorf("log record pid = %v, want %v", rec["pid"], wantPID)
		}
		// Assert path attribute contains the session file path.
		if rec["path"] != path {
			t.Errorf("log record path = %q, want %q", rec["path"], path)
		}
		break
	}
	if !found {
		t.Errorf("expected slog WARN record %q, none found in captured output:\n%s",
			"session locked by concurrent process", logBuf.String())
	}
}

// TestDiskStore_ConcurrentUpdate_AtLeastOneSucceeds ensures goroutine-level concurrent
// writes are serialised (no data loss, no panic) and at least one succeeds.
func TestDiskStore_ConcurrentUpdate_AtLeastOneSucceeds(t *testing.T) {
	store, _ := newTestDiskStore(t)
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd for concurrent test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var wg sync.WaitGroup
	const workers = 5
	results := make([]error, workers)

	for i := range workers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			clone := *sess
			clone.State = sessionstore.StateScored
			results[idx] = store.Update(ctx, &clone)
		}(i)
	}
	wg.Wait()

	successCount := 0
	for _, r := range results {
		if r == nil {
			successCount++
		}
	}
	if successCount == 0 {
		t.Error("all concurrent updates failed; at least one must succeed")
	}
}

// TestDiskStore_SessionID_Entropy verifies that 10 consecutive DiskStore.Create
// calls yield 10 distinct IDs, each matching ^[a-z0-9]{26,64}$.
// This is a probabilistic collision test; a collision here would indicate a broken
// CSPRNG and is vanishingly unlikely (~1/2^127 per pair).
func TestDiskStore_SessionID_Entropy(t *testing.T) {
	store, _ := newTestDiskStore(t)
	ctx := context.Background()

	const n = 10
	seen := make(map[string]struct{}, n)

	for i := range n {
		sess, err := store.Create(ctx, "entropy test jd")
		if err != nil {
			t.Fatalf("Create #%d: %v", i, err)
		}
		if !sessionIDRegex.MatchString(sess.ID) {
			t.Errorf("session ID %q does not match regex ^[a-z0-9]{26,64}$", sess.ID)
		}
		if _, dup := seen[sess.ID]; dup {
			t.Fatalf("duplicate session ID %q generated (entropy failure)", sess.ID)
		}
		seen[sess.ID] = struct{}{}
	}

	if len(seen) != n {
		t.Errorf("expected %d distinct IDs; got %d", n, len(seen))
	}
}

// TestDiskStore_Update_NoTruncateOnContention verifies that a failed Update (due to
// lock contention) does NOT corrupt the existing session file on disk.
// The file must be byte-identical to its pre-Update contents after the contending
// Update returns ErrSessionLocked.
func TestDiskStore_Update_NoTruncateOnContention(t *testing.T) {
	store, sessDir := newTestDiskStore(t)
	ctx := context.Background()

	// Create a session and capture its serialised form.
	sess, err := store.Create(ctx, "jd for truncation guard test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	path := filepath.Join(sessDir, sess.ID+".json")
	originalBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original file: %v", err)
	}
	if len(originalBytes) == 0 {
		t.Fatal("original session file is empty — unexpected")
	}

	// Hold an exclusive flock on a separate FD to simulate contention.
	holder, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open holder fd: %v", err)
	}
	defer holder.Close() //nolint:errcheck

	if lockErr := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); lockErr != nil {
		t.Skipf("flock unavailable on this platform/fs: %v", lockErr)
	}

	// Attempt Update while lock is held — must return ErrSessionLocked.
	clone := *sess
	clone.State = sessionstore.StateScored
	updateErr := store.Update(ctx, &clone)

	// Release the holder lock.
	_ = syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)

	if !errors.Is(updateErr, sessionstore.ErrSessionLocked) {
		t.Fatalf("expected ErrSessionLocked, got: %v", updateErr)
	}

	// The on-disk file MUST be unchanged — not truncated, not corrupted.
	afterBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file after contended Update: %v", err)
	}
	if !bytes.Equal(originalBytes, afterBytes) {
		t.Errorf("file was modified despite ErrSessionLocked\nbefore (%d bytes): %s\nafter  (%d bytes): %s",
			len(originalBytes), originalBytes, len(afterBytes), afterBytes)
	}
}

// TestDiskStore_Get_DuringUpdate_ReturnsPriorOrNewState verifies that concurrent
// Get calls during an Update never observe a torn/partial file.
// With atomic rename, Get sees either the old complete file or the new complete one.
func TestDiskStore_Get_DuringUpdate_ReturnsPriorOrNewState(t *testing.T) {
	store, _ := newTestDiskStore(t)
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd for concurrent get/update test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	const iterations = 50
	var wg sync.WaitGroup

	// Concurrent updaters.
	for i := range iterations {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			clone := *sess
			if i%2 == 0 {
				clone.State = sessionstore.StateScored
			} else {
				clone.State = sessionstore.StateTailored
			}
			_ = store.Update(ctx, &clone) // ignore lock contention errors
		}(i)
	}

	// Concurrent readers — must never see a decode error.
	for range iterations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _, err := store.Get(ctx, sess.ID)
			if err != nil {
				t.Errorf("Get during concurrent Update returned error: %v", err)
			}
			if got != nil {
				switch got.State {
				case sessionstore.StateLoaded, sessionstore.StateScored, sessionstore.StateTailored:
					// all valid states the file may be in
				default:
					t.Errorf("Get returned session with unexpected state %q", got.State)
				}
			}
		}()
	}

	wg.Wait()
}

func TestDiskStore_SessionFile_ValidJSON(t *testing.T) {
	store, sessDir := newTestDiskStore(t)
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd text")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	path := filepath.Join(sessDir, sess.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("session file is not valid JSON: %v", err)
	}

	// Required fields must be present.
	for _, field := range []string{"id", "state", "jd_text", "created_at"} {
		if _, ok := raw[field]; !ok {
			t.Errorf("session JSON missing field %q", field)
		}
	}
}
