// Package mcp provides a capturing Presenter for MCP tool handlers.
// Instead of writing to stdout/stderr, it accumulates pipeline results in memory
// so that tool handlers can read them after pipeline.Run completes.
package mcp

import (
	"sync"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Presenter = (*Presenter)(nil)

// Presenter captures pipeline output in memory for MCP tool handlers.
// It is NOT concurrent-safe across multiple goroutines — create a new instance
// per tool invocation. The mutex guards against incidental concurrent writes
// within a single pipeline run.
type Presenter struct {
	mu     sync.Mutex
	result *model.PipelineResult
	tailor *model.TailorResult
	err    error
	events []any
}

// New returns a new capturing Presenter.
func New() *Presenter {
	return &Presenter{}
}

// OnEvent records a pipeline step lifecycle event.
// Unknown event types are silently ignored and never cause a panic.
func (p *Presenter) OnEvent(event any) {
	if event == nil {
		return
	}
	switch event.(type) {
	case model.StepStartedEvent, model.StepCompletedEvent, model.StepFailedEvent:
		p.mu.Lock()
		p.events = append(p.events, event)
		p.mu.Unlock()
	}
}

// ShowResult stores the pipeline result in memory.
func (p *Presenter) ShowResult(result *model.PipelineResult) error {
	p.mu.Lock()
	p.result = result
	p.mu.Unlock()
	return nil
}

// ShowTailorResult stores the tailor result in memory.
func (p *Presenter) ShowTailorResult(result *model.TailorResult) error {
	p.mu.Lock()
	p.tailor = result
	p.mu.Unlock()
	return nil
}

// ShowError stores the error in memory.
func (p *Presenter) ShowError(err error) {
	p.mu.Lock()
	p.err = err
	p.mu.Unlock()
}

// Result returns the captured PipelineResult, or nil if the pipeline failed fatally.
func (p *Presenter) Result() *model.PipelineResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result
}

// TailorResult returns the captured TailorResult, or nil if none was produced.
func (p *Presenter) TailorResult() *model.TailorResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.tailor
}

// Err returns the captured error from ShowError, or nil.
func (p *Presenter) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}
