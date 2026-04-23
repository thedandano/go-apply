package port

import "github.com/thedandano/go-apply/internal/model"

// Presenter receives pipeline events and results.
// Implementations: headless.JSONPresenter, mcp.MCPPresenter.
// Pipeline services call into Presenter — never the reverse.
type Presenter interface {
	// OnEvent is called for each step lifecycle event.
	// event is one of: model.StepStartedEvent, model.StepCompletedEvent, model.StepFailedEvent
	OnEvent(event any)
	ShowResult(result *model.PipelineResult) error
	ShowTailorResult(result *model.TailorResult) error
	ShowError(err error)
}
