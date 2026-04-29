package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// sessionState represents the position of a session in the multi-turn state machine.
type sessionState int

const (
	stateLoaded    sessionState = iota // load_jd succeeded; host must provide keywords
	stateScored                        // submit_keywords succeeded; host decides next step
	stateT1Applied                     // submit_tailor_t1 succeeded
	stateT2Applied                     // submit_tailor_t2 succeeded
	stateFinalized                     // finalize succeeded
)

func (s sessionState) String() string {
	switch s {
	case stateLoaded:
		return "loaded"
	case stateScored:
		return "scored"
	case stateT1Applied:
		return "t1_applied"
	case stateT2Applied:
		return "t2_applied"
	case stateFinalized:
		return "finalized"
	default:
		return "unknown"
	}
}

// Session holds the in-flight state for one multi-turn job application conversation.
// Sessions are persisted to disk under {dataDir}/sessions/{id}.json.
type Session struct {
	ID               string                     `json:"id"`
	State            sessionState               `json:"state"`
	URL              string                     `json:"url"` // original URL if load_jd was called with jd_url; empty for text input
	IsText           bool                       `json:"is_text"`
	JDText           string                     `json:"jd_text"`
	JD               model.JDData               `json:"jd"`
	ScoreResult      pipeline.ScoreResumeResult `json:"score_result"`
	TailoredText     string                     `json:"tailored_text"` // updated by T1/T2 handlers
	TailoredSections *model.SectionMap          `json:"tailored_sections"`
	CreatedAt        time.Time                  `json:"created_at"`
}

// SessionStore persists sessions to disk; one file per session.
// File path: {dataDir}/sessions/{id}.json
// No in-memory cache; each operation reads from or writes to disk.
type SessionStore struct {
	dataDir string
}

// NewSessionStore creates a SessionStore backed by the given data directory.
func NewSessionStore(dataDir string) *SessionStore {
	return &SessionStore{dataDir: dataDir}
}

func (s *SessionStore) sessionsDir() string {
	return filepath.Join(s.dataDir, "sessions")
}

func (s *SessionStore) filePath(id string) string {
	return filepath.Join(s.sessionsDir(), id+".json")
}

// Create mints a new Session with a random ID, stateLoaded, and the given JD text.
// The session is NOT persisted; callers must call Save after any field mutations.
func (s *SessionStore) Create(jdText string) *Session {
	return &Session{
		ID:        newSessionID(),
		State:     stateLoaded,
		JDText:    jdText,
		CreatedAt: time.Now().UTC(),
	}
}

// Save atomically writes the session to disk (temp file + rename).
// Permissions: 0600. Logs ERROR on failure, DEBUG on success.
func (s *SessionStore) Save(sess *Session) error {
	dir := s.sessionsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		slog.ErrorContext(context.Background(), "session: create sessions dir failed",
			slog.String("session_id", sess.ID), slog.String("error", err.Error()))
		return fmt.Errorf("create sessions dir: %w", err)
	}
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	dst := s.filePath(sess.ID)
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil { // #nosec G306
		slog.ErrorContext(context.Background(), "session: write failed",
			slog.String("session_id", sess.ID), slog.String("error", err.Error()))
		return fmt.Errorf("write session tmp: %w", err)
	}
	defer func() { _ = os.Remove(tmp) }() // cleans up tmp on panic or early return
	if err := os.Rename(tmp, dst); err != nil {
		slog.ErrorContext(context.Background(), "session: rename failed",
			slog.String("session_id", sess.ID), slog.String("error", err.Error()))
		return fmt.Errorf("rename session file: %w", err)
	}
	slog.DebugContext(context.Background(), "session: saved",
		slog.String("session_id", sess.ID), slog.String("state", sess.State.String()))
	return nil
}

// Load reads a session from disk by ID.
// Returns error if the file is missing or cannot be parsed.
func (s *SessionStore) Load(id string) (*Session, error) {
	data, err := os.ReadFile(s.filePath(id)) // #nosec G304
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		slog.ErrorContext(context.Background(), "session: read failed",
			slog.String("session_id", id), slog.String("error", err.Error()))
		return nil, fmt.Errorf("read session: %w", err)
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		slog.ErrorContext(context.Background(), "session: parse failed (corrupt file)",
			slog.String("session_id", id), slog.String("error", err.Error()))
		return nil, fmt.Errorf("session not found: %s", id)
	}
	return &sess, nil
}

// Delete removes the session file. Called on finalize.
// Logs ERROR on failure, DEBUG on success.
func (s *SessionStore) Delete(id string) error {
	if err := os.Remove(s.filePath(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.ErrorContext(context.Background(), "session: delete failed",
			slog.String("session_id", id), slog.String("error", err.Error()))
		return fmt.Errorf("delete session: %w", err)
	}
	slog.DebugContext(context.Background(), "session: deleted", slog.String("session_id", id))
	return nil
}

// SweepExpired deletes session files older than maxAge based on file mtime.
// Logs INFO with the count of deleted files (FR-021).
func (s *SessionStore) SweepExpired(maxAge time.Duration) {
	pattern := filepath.Join(s.sessionsDir(), "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	deleted := 0
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(f); err == nil {
				deleted++
			}
		}
	}
	slog.InfoContext(context.Background(), "session: swept expired files",
		slog.Int("deleted", deleted), slog.String("data_dir", s.dataDir))
}

// newSessionID generates a random 16-byte hex session identifier.
func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("session ID generation failed: %v", err))
	}
	return fmt.Sprintf("%x", b)
}
