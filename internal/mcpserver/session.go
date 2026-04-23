package mcpserver

import (
	"crypto/rand"
	"fmt"
	"sync"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// sessionState represents the position of a session in the multi-turn state machine.
type sessionState int

const (
	stateLoaded    sessionState = iota // load_jd succeeded; host must provide keywords
	stateScored                        // submit_keywords succeeded; host decides next step
	stateTailored                      // agent submitted tailored resume; host may finalize
	stateFinalized                     // finalize succeeded
)

func (s sessionState) String() string {
	switch s {
	case stateLoaded:
		return "loaded"
	case stateScored:
		return "scored"
	case stateTailored:
		return "tailored"
	case stateFinalized:
		return "finalized"
	default:
		return "unknown"
	}
}

// Session holds the in-flight state for one multi-turn job application conversation.
// Sessions are ephemeral: lost when the MCP server process exits.
type Session struct {
	ID           string
	State        sessionState
	URL          string // original URL if load_jd was called with jd_url; empty for text input
	IsText       bool
	JDText       string
	JD           model.JDData
	ScoreResult  pipeline.ScoreResumeResult
	TailoredText string                 // reserved for agent-submitted tailored resume text (Unit 3)
	Changelog    []model.ChangelogEntry // tailoring actions recorded by the agent (Unit 3)
}

const sessionStoreCap = 100

// SessionStore is an in-memory store for active sessions with LRU eviction.
// Lifetime matches the MCP server process; no disk persistence.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
	order    []string // keys ordered oldest-first; newest is last
}

// NewSessionStore creates an empty SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session, sessionStoreCap),
		order:    make([]string, 0, sessionStoreCap),
	}
}

// Create mints a new session, evicting the oldest entry when the store is full.
func (s *SessionStore) Create(jdText string) *Session {
	id := newSessionID()
	sess := &Session{ID: id, State: stateLoaded, JDText: jdText}

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.sessions) >= sessionStoreCap {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.sessions, oldest)
	}
	s.sessions[id] = sess
	s.order = append(s.order, id)
	return sess
}

// Get returns the session for the given ID, moving it to the front of the LRU order.
// Returns nil if the session does not exist.
func (s *SessionStore) Get(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil
	}
	s.touch(id)
	return sess
}

// touch moves the given key to the back of the LRU order (most recently used).
// Must be called with s.mu held.
func (s *SessionStore) touch(id string) {
	for i, k := range s.order {
		if k == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			s.order = append(s.order, id)
			return
		}
	}
}

// newSessionID generates a random 16-byte hex session identifier.
func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("session ID generation failed: %v", err))
	}
	return fmt.Sprintf("%x", b)
}
