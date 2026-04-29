// Package mcp provides a capturing presenter for MCP tool handler contexts.
package mcp

import "github.com/thedandano/go-apply/internal/port"

// Compile-time interface check.
var _ port.Presenter = (*CapturingPresenter)(nil)

// CapturingPresenter satisfies the Presenter interface for MCP tool handlers.
type CapturingPresenter struct{}

// New constructs a CapturingPresenter.
func New() *CapturingPresenter {
	return &CapturingPresenter{}
}

// OnEvent is a no-op — MCP tools do not stream progress events.
func (p *CapturingPresenter) OnEvent(_ any) {}
