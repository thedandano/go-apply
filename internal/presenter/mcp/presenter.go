// Package mcp provides a capturing presenter for MCP tool handler contexts.
// It accumulates pipeline results in memory so tool handlers can read them back
// after pipeline.Run() returns, instead of streaming them to stdout.
package mcp

import (
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface check.
var _ port.Presenter = (*CapturingPresenter)(nil)

// CapturingPresenter captures pipeline results in memory.
// MCP tool handlers run pipeline.Run(), then read Result and TailorResult back.
type CapturingPresenter struct {
	Result       *model.PipelineResult
	TailorResult *model.TailorResult
}

// New constructs a CapturingPresenter with nil result fields.
func New() *CapturingPresenter {
	return &CapturingPresenter{}
}

// OnEvent is a no-op — MCP tools do not stream progress events.
func (p *CapturingPresenter) OnEvent(_ any) {}

// ShowResult stores the pipeline result pointer and returns nil.
func (p *CapturingPresenter) ShowResult(result *model.PipelineResult) error {
	p.Result = result
	return nil
}

// ShowTailorResult stores the tailor result pointer and returns nil.
func (p *CapturingPresenter) ShowTailorResult(result *model.TailorResult) error {
	p.TailorResult = result
	return nil
}

// ShowError is a no-op — MCP handlers return errors via the pipeline.Run() return value.
func (p *CapturingPresenter) ShowError(_ error) {}
