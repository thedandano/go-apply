package mcp_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	mcppres "github.com/thedandano/go-apply/internal/presenter/mcp"
)

func TestCapturingPresenter_OnEvent_IsNoop(t *testing.T) {
	p := mcppres.New()

	// Must not panic with any event type.
	p.OnEvent(model.StepStartedEvent{StepID: "01", Label: "Test"})
	p.OnEvent(model.StepCompletedEvent{StepID: "01", Label: "Test", ElapsedMS: 100})
	p.OnEvent(model.StepFailedEvent{StepID: "01", Label: "Test", Err: "boom"})
	p.OnEvent("unknown event type")
	p.OnEvent(nil)
}
