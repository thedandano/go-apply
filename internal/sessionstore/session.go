// Package sessionstore defines the Session type and the Store interface used by
// both the MCP server (in-memory) and CLI subcommands (disk-backed).
package sessionstore

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// State represents the position of a session in the multi-turn state machine.
// It is stored as a string so JSON round-trips are stable regardless of const ordering.
type State string

const (
	StateLoaded    State = "loaded"
	StateScored    State = "scored"
	StateTailored  State = "tailored"
	StateFinalized State = "finalized"
)

// Session holds the in-flight state for one multi-turn job application conversation.
type Session struct {
	ID           string                     `json:"id"`
	State        State                      `json:"state"`
	URL          string                     `json:"url,omitempty"`
	IsText       bool                       `json:"is_text"`
	JDText       string                     `json:"jd_text"`
	JD           model.JDData               `json:"jd"`
	ScoreResult  pipeline.ScoreResumeResult `json:"score_result"`
	TailoredText string                     `json:"tailored_text,omitempty"`
	Changelog    []model.ChangelogEntry     `json:"changelog,omitempty"`
	CreatedAt    time.Time                  `json:"created_at"`
}

// sessionAlias is used in UnmarshalJSON to avoid infinite recursion.
type sessionAlias Session

// UnmarshalJSON decodes a session from JSON and validates the state field.
func (s *Session) UnmarshalJSON(data []byte) error {
	var a sessionAlias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*s = Session(a)
	return s.validate()
}

// validate checks that the State field holds a known value after JSON decode.
func (s *Session) validate() error {
	switch s.State {
	case StateLoaded, StateScored, StateTailored, StateFinalized:
		return nil
	default:
		return fmt.Errorf("unknown session state %q", s.State)
	}
}
