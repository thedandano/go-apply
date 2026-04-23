package sessionstore

import "context"

// Store is the interface for session lifecycle management.
// Create, Get, Update, and Delete are the four operations that session handlers need.
// Both in-memory (MCP) and disk-backed (CLI) implementations satisfy this interface.
type Store interface {
	// Create mints a new session with the given JD text and returns it.
	Create(ctx context.Context, jdText string) (*Session, error)
	// Get returns the session for id and true when found; returns nil, false, nil when not found.
	Get(ctx context.Context, id string) (*Session, bool, error)
	// Update persists changes to an existing session.
	Update(ctx context.Context, sess *Session) error
	// Delete removes the session with the given id.
	Delete(ctx context.Context, id string) error
}
