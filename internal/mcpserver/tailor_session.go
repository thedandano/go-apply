package mcpserver

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/thedandano/go-apply/internal/model"
)

// Sentinel errors for TailorSessionStore operations.
var (
	ErrTailorSessionUnknown         = errors.New("tailor session not found")
	ErrTailorSessionExpired         = errors.New("tailor session expired")
	ErrTailorSessionAlreadyConsumed = errors.New("tailor session already consumed")
)

// TailorSession holds the in-flight state for a single LLM tailor round-trip.
type TailorSession struct {
	ID           string
	PromptBundle string
	Input        *model.TailorInput

	result    *model.TailorResult
	resultErr error

	done      chan struct{}
	expiresAt time.Time
	timer     *time.Timer
}

// TailorSessionStore is a concurrent-safe in-memory store for active TailorSessions.
type TailorSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*TailorSession
}

// NewTailorSessionStore creates an empty TailorSessionStore.
func NewTailorSessionStore() *TailorSessionStore {
	return &TailorSessionStore{
		sessions: make(map[string]*TailorSession),
	}
}

// Open creates a new TailorSession with the given timeout, stores it, then starts its
// timer. The session is stored before the timer starts to eliminate the bootstrap race
// where the timer fires before the session is in the map. Returns the opaque session ID.
func (s *TailorSessionStore) Open(promptBundle string, input *model.TailorInput, timeout time.Duration) (string, error) {
	id := newTailorSessionID()
	sess := &TailorSession{
		ID:           id,
		PromptBundle: promptBundle,
		Input:        input,
		done:         make(chan struct{}),
		expiresAt:    time.Now().Add(timeout),
	}

	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()

	// AfterFunc receives the session pointer directly to avoid a map lookup
	// race if the timer fires before Open returns.
	sess.timer = time.AfterFunc(timeout, func() { s.expireSession(sess) })

	return id, nil
}

// expireSession closes the session's done channel and marks it expired.
// Takes *TailorSession directly to avoid a bootstrap race with Open.
func (s *TailorSessionStore) expireSession(sess *TailorSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-sess.done:
		return // already submitted or expired
	default:
	}
	sess.resultErr = ErrTailorSessionExpired
	close(sess.done)
}

// Submit delivers the LLM result to the session identified by id. Returns
// ErrTailorSessionUnknown if id is not found, ErrTailorSessionExpired if the
// session already timed out, or ErrTailorSessionAlreadyConsumed if Submit was
// already called successfully.
func (s *TailorSessionStore) Submit(id string, result *model.TailorResult) error {
	s.mu.Lock()

	sess, ok := s.sessions[id]
	if !ok {
		s.mu.Unlock()
		return ErrTailorSessionUnknown
	}

	select {
	case <-sess.done:
		err := sess.resultErr
		s.mu.Unlock()
		if errors.Is(err, ErrTailorSessionExpired) {
			return ErrTailorSessionExpired
		}
		return ErrTailorSessionAlreadyConsumed
	default:
	}

	sess.timer.Stop()
	sess.result = result
	close(sess.done)
	s.mu.Unlock()

	return nil
}

// Wait blocks until Submit delivers a result or ctx is cancelled. Returns
// ErrTailorSessionUnknown if id is not found, ErrTailorSessionExpired if the
// session timed out while waiting.
func (s *TailorSessionStore) Wait(ctx context.Context, id string) (model.TailorResult, error) {
	s.mu.Lock()
	sess, ok := s.sessions[id]
	if !ok {
		s.mu.Unlock()
		return model.TailorResult{}, ErrTailorSessionUnknown
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		return model.TailorResult{}, ctx.Err()
	case <-sess.done:
	}

	s.mu.Lock()
	result := sess.result
	resultErr := sess.resultErr
	s.mu.Unlock()

	if resultErr != nil {
		return model.TailorResult{}, resultErr
	}
	return *result, nil
}

// newTailorSessionID generates a random 16-byte hex session identifier.
func newTailorSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("tailor session ID generation failed: %v", err))
	}
	return fmt.Sprintf("%x", b)
}
