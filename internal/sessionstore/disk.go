package sessionstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// writeSession creates (isCreate=true) or atomically replaces (isCreate=false) the session file.
//
// Create path: opens with O_WRONLY|O_CREATE|O_EXCL — file must not exist. EEXIST is
// returned as ErrSessionExists (astronomically rare with crypto/rand IDs but surfaced).
// Acquires an exclusive flock on the new file before writing.
//
// Update path (atomic temp + rename): writes to a unique <path>.tmp.<nonce> file first,
// then acquires an exclusive flock on the ORIGINAL path (opened without O_TRUNC so the
// existing content is never destroyed before the lock is confirmed), renames the tmp
// over the original only after the lock is held, and releases the lock by closing the
// original FD. The rename is atomic on the same filesystem, so concurrent readers see
// either the old complete file or the new one — never a partial write.
//
// On contention (EWOULDBLOCK): the tmp file is removed and ErrSessionLocked is returned;
// the original file is untouched.
//
// An explicit Chmod to 0o600 is applied to the new/tmp file to override a paranoid umask.
func (d *DiskStore) writeSession(ctx context.Context, sess *Session, isCreate bool) error {
	path := d.sessionPath(sess.ID)

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session %q: %w", sess.ID, err)
	}

	if isCreate {
		return d.writeSessionCreate(ctx, sess.ID, path, data)
	}
	return d.writeSessionUpdate(ctx, sess.ID, path, data)
}

// writeSessionCreate handles the Create path: opens with O_EXCL to prevent overwrites,
// locks the new file, then writes.
func (d *DiskStore) writeSessionCreate(ctx context.Context, id, path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", ErrSessionExists, id)
		}
		return fmt.Errorf("open session file %q: %w", id, err)
	}
	defer f.Close() //nolint:errcheck

	if chErr := os.Chmod(path, 0o600); chErr != nil {
		return fmt.Errorf("chmod session file %q: %w", id, chErr)
	}

	if lockErr := flockExclusive(f); lockErr != nil {
		if errors.Is(lockErr, syscall.EWOULDBLOCK) {
			slog.WarnContext(ctx, "session locked by concurrent process",
				"session_id", id,
				"pid", os.Getpid(),
				"path", path,
			)
			return fmt.Errorf("%w: %s", ErrSessionLocked, id)
		}
		return fmt.Errorf("lock session file %q: %w", id, lockErr)
	}

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write session %q: %w", id, err)
	}
	return nil
}

// writeSessionUpdate handles the Update path atomically:
// 1. Write payload to a unique tmp file.
// 2. Acquire exclusive flock on the ORIGINAL file (no O_TRUNC).
// 3. If lock fails with EWOULDBLOCK: remove tmp, return ErrSessionLocked.
// 4. On success: rename tmp → original (atomic), then release lock.
func (d *DiskStore) writeSessionUpdate(ctx context.Context, id, path string, data []byte) error {
	// Generate a unique tmp file path in the same directory.
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("generate tmp nonce for session %q: %w", id, err)
	}
	tmpPath := fmt.Sprintf("%s.tmp.%d.%s", path, os.Getpid(), hex.EncodeToString(nonce[:]))

	// Write to tmp first. Clean it up on any non-committed exit.
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	tmpF, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("open tmp file for session %q: %w", id, err)
	}

	if chErr := os.Chmod(tmpPath, 0o600); chErr != nil {
		tmpF.Close() //nolint:errcheck
		return fmt.Errorf("chmod tmp file for session %q: %w", id, chErr)
	}

	if _, writeErr := tmpF.Write(data); writeErr != nil {
		tmpF.Close() //nolint:errcheck
		return fmt.Errorf("write tmp file for session %q: %w", id, writeErr)
	}
	if syncErr := tmpF.Sync(); syncErr != nil {
		tmpF.Close() //nolint:errcheck
		return fmt.Errorf("sync tmp file for session %q: %w", id, syncErr)
	}
	tmpF.Close() //nolint:errcheck

	// Acquire exclusive flock on the original file WITHOUT truncating it.
	// O_RDONLY is sufficient for flock semantics and does not require write permission.
	// If the file does not exist, Update semantics require an error (Create must precede Update).
	origF, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("open session file %q: %w", id, err)
		}
		return fmt.Errorf("open session file %q: %w", id, err)
	}
	defer origF.Close() //nolint:errcheck

	if lockErr := flockExclusive(origF); lockErr != nil {
		if errors.Is(lockErr, syscall.EWOULDBLOCK) {
			slog.WarnContext(ctx, "session locked by concurrent process",
				"session_id", id,
				"pid", os.Getpid(),
				"path", path,
			)
			return fmt.Errorf("%w: %s", ErrSessionLocked, id)
		}
		return fmt.Errorf("lock session file %q: %w", id, lockErr)
	}

	// Lock is held. Rename tmp → original (atomic on same FS).
	if renErr := os.Rename(tmpPath, path); renErr != nil {
		return fmt.Errorf("rename tmp to session file %q: %w", id, renErr)
	}
	committed = true
	// origF.Close() via defer releases the flock.
	return nil
}

// flockExclusive acquires a non-blocking exclusive advisory lock on f.
// Returns syscall.EWOULDBLOCK if the file is already locked by another process.
func flockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
