package sessionstore

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

const memoryCap = 100

// MemoryStore is an in-memory Store with LRU eviction.
// Lifetime matches the process; no disk persistence.
// Used by the MCP server.
type MemoryStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
	order    []string // keys ordered oldest-first; newest is last
}

// NewMemoryStore creates an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions: make(map[string]*Session, memoryCap),
		order:    make([]string, 0, memoryCap),
	}
}

// Create mints a new session, evicting the oldest entry when the store is full.
func (s *MemoryStore) Create(_ context.Context, jdText string) (*Session, error) {
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

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.sessions) >= memoryCap {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.sessions, oldest)
	}
	s.sessions[id] = sess
	s.order = append(s.order, id)
	return sess, nil
}

// Get returns the session for the given id.
// Returns nil, false, nil when not found.
func (s *MemoryStore) Get(_ context.Context, id string) (*Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil, false, nil
	}
	s.touch(id)
	return sess, true, nil
}

// Update persists changes to an existing session.
// For MemoryStore the session pointer is already live so this is a no-op, but it
// satisfies the Store interface so handler code written for DiskStore works here too.
func (s *MemoryStore) Update(_ context.Context, sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sess.ID]; !ok {
		return fmt.Errorf("session %q not found", sess.ID)
	}
	s.sessions[sess.ID] = sess
	s.touch(sess.ID)
	return nil
}

// Delete removes the session with the given id.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return fmt.Errorf("session %q not found", id)
	}
	delete(s.sessions, id)
	for i, k := range s.order {
		if k == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return nil
}

// touch moves the given key to the back of the LRU order (most recently used).
// Must be called with s.mu held.
func (s *MemoryStore) touch(id string) {
	for i, k := range s.order {
		if k == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			s.order = append(s.order, id)
			return
		}
	}
}

// newSessionID generates a random 16-byte (128-bit) hex session identifier.
// The result matches ^[a-z0-9]{32}$ which satisfies the spec regex ^[a-z0-9]{26,64}$.
func newSessionID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}
