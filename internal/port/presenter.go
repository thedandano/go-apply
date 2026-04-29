package port

// Presenter receives pipeline events.
// Implementation: mcp.CapturingPresenter.
// Pipeline services call into Presenter — never the reverse.
type Presenter interface {
	// OnEvent is called for each step lifecycle event.
	// event is one of: model.StepStartedEvent, model.StepCompletedEvent, model.StepFailedEvent
	OnEvent(event any)
}
