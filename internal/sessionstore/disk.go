package sessionstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ErrSessionLocked is returned when a concurrent process holds the advisory lock
// on a session file.
var ErrSessionLocked = errors.New("session_locked")

// ErrSessionExists is returned when a Create call finds the target file already
// exists on disk. This is astronomically rare with crypto/rand IDs but can occur
// if two concurrent Creates collide or a file was left behind by a previous run.
var ErrSessionExists = errors.New("session_exists")

// DiskStore is a file-per-session JSON store.
// Session files live at <dir>/<id>.json.
// Directory permissions: 0o700. File permissions: 0o600.
// Advisory file locking (syscall.Flock) is used on all writes.
type DiskStore struct {
	dir string // absolute path to the sessions directory
}

// NewDiskStore creates a DiskStore rooted at dir.
// The directory (and all parents) are created if absent, with permissions 0o700.
// An explicit Chmod is applied after creation to override a loose umask.
func NewDiskStore(dir string) (*DiskStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("chmod sessions dir: %w", err)
	}
	return &DiskStore{dir: dir}, nil
}

// Create mints a new session file.
func (d *DiskStore) Create(ctx context.Context, jdText string) (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}

	sess := &Session{
		ID:        id,
		State:     StateLoaded,
		JDText:    jdText,
		CreatedAt: time.Now().UTC(),
	}

	if err := d.writeSession(ctx, sess, true); err != nil {
		return nil, err
	}
	return sess, nil
}

// Get reads the session from disk. Returns nil, false, nil when not found.
func (d *DiskStore) Get(_ context.Context, id string) (*Session, bool, error) {
	path := d.sessionPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read session %q: %w", id, err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, false, fmt.Errorf("decode session %q: %w", id, err)
	}
	return &sess, true, nil
}

// Update writes updated session data to disk, replacing the existing file.
// Acquires an advisory exclusive lock before writing. Returns ErrSessionLocked
// on contention and emits slog.WarnContext.
func (d *DiskStore) Update(ctx context.Context, sess *Session) error {
	return d.writeSession(ctx, sess, false)
}

// Delete removes the session file. Returns nil if the file does not exist (idempotent).
func (d *DiskStore) Delete(_ context.Context, id string) error {
	path := d.sessionPath(id)
	err := os.Remove(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete session %q: %w", id, err)
	}
	return nil
}

// sessionPath returns the file path for a session id.
func (d *DiskStore) sessionPath(id string) string {
	return filepath.Join(d.dir, id+".json")
}

// writeSession creates (isCreate=true) or replaces (isCreate=false) the session file.
// Create path: O_WRONLY|O_CREATE|O_EXCL — file must not exist; EEXIST is returned as
// ErrSessionExists (astronomically rare with crypto/rand IDs but correctly surfaced).
// Update path: O_WRONLY|O_TRUNC — file must already exist; no O_CREATE so open fails
// if the file was deleted (correct Update semantics).
// An advisory exclusive non-blocking flock is acquired before writing;
// on contention ErrSessionLocked is returned.
// An explicit Chmod to 0o600 is applied after opening to override a paranoid umask.
func (d *DiskStore) writeSession(ctx context.Context, sess *Session, isCreate bool) error {
	path := d.sessionPath(sess.ID)

	var flags int
	if isCreate {
		// O_EXCL: fail if the file already exists — prevents silent overwrites on ID
		// collisions and ensures exactly-once creation semantics.
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	} else {
		// No O_CREATE: if the file is absent the open fails fast, which is the
		// correct behaviour for an Update (caller must Create first).
		flags = os.O_WRONLY | os.O_TRUNC
	}

	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		if isCreate && errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", ErrSessionExists, sess.ID)
		}
		return fmt.Errorf("open session file %q: %w", sess.ID, err)
	}
	defer f.Close() //nolint:errcheck

	// Explicit chmod to override any umask that may have restricted the mode set
	// in OpenFile (e.g. umask 0o177 would yield 0o400). Applied unconditionally on
	// both Create and Update paths.
	if chErr := os.Chmod(path, 0o600); chErr != nil {
		return fmt.Errorf("chmod session file %q: %w", sess.ID, chErr)
	}

	// Acquire advisory exclusive non-blocking lock.
	if lockErr := flockExclusive(f); lockErr != nil {
		if errors.Is(lockErr, syscall.EWOULDBLOCK) {
			slog.WarnContext(ctx, "session locked by concurrent process",
				"session_id", sess.ID,
				"pid", os.Getpid(),
				"path", path,
			)
			return fmt.Errorf("%w: %s", ErrSessionLocked, sess.ID)
		}
		return fmt.Errorf("lock session file %q: %w", sess.ID, lockErr)
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session %q: %w", sess.ID, err)
	}

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write session %q: %w", sess.ID, err)
	}

	return nil
}

// flockExclusive acquires a non-blocking exclusive advisory lock on f.
// Returns syscall.EWOULDBLOCK if the file is already locked by another process.
func flockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
