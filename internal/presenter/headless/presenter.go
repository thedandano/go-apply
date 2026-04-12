// Package headless provides a JSON-streaming Presenter for non-interactive (agent/CI) use.
// Step lifecycle events are written to stderr as newline-delimited JSON objects.
// Final results and errors are written to the configured out writer (typically stdout).
package headless

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.Presenter = (*Presenter)(nil)

// Presenter writes pipeline events as newline-delimited JSON to stderr and
// final results to out (stdout by default).
type Presenter struct {
	out    io.Writer
	events io.Writer
}

// New returns a Presenter that writes results to out and events to stderr.
func New(out io.Writer) *Presenter {
	return &Presenter{
		out:    out,
		events: os.Stderr,
	}
}

// NewWithEventWriter returns a Presenter that writes results to out and events
// to the provided events writer. Useful for tests.
func NewWithEventWriter(out io.Writer, events io.Writer) *Presenter {
	return &Presenter{
		out:    out,
		events: events,
	}
}

type stepStartedJSON struct {
	Event  string `json:"event"`
	StepID string `json:"step_id"`
	Label  string `json:"label"`
}

type stepCompletedJSON struct {
	Event     string `json:"event"`
	StepID    string `json:"step_id"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type stepFailedJSON struct {
	Event  string `json:"event"`
	StepID string `json:"step_id"`
	Error  string `json:"error"`
}

// OnEvent writes a JSON step lifecycle event to stderr.
// Unrecognised event types are silently ignored.
func (p *Presenter) OnEvent(event any) {
	switch e := event.(type) {
	case model.StepStartedEvent:
		p.writeJSON(p.events, stepStartedJSON{
			Event:  "step_started",
			StepID: e.StepID,
			Label:  e.Label,
		})
	case model.StepCompletedEvent:
		p.writeJSON(p.events, stepCompletedJSON{
			Event:     "step_completed",
			StepID:    e.StepID,
			ElapsedMS: e.ElapsedMS,
		})
	case model.StepFailedEvent:
		p.writeJSON(p.events, stepFailedJSON{
			Event:  "step_failed",
			StepID: e.StepID,
			Error:  e.Err,
		})
	}
}

// ShowResult marshals result to JSON and writes it to p.out followed by a newline.
func (p *Presenter) ShowResult(result *model.PipelineResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("headless: marshal pipeline result: %w", err)
	}
	_, err = fmt.Fprintf(p.out, "%s\n", data)
	return err
}

// ShowTailorResult marshals result to JSON and writes it to p.out followed by a newline.
func (p *Presenter) ShowTailorResult(result *model.TailorResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("headless: marshal tailor result: %w", err)
	}
	_, err = fmt.Fprintf(p.out, "%s\n", data)
	return err
}

// ShowError writes {"error":"<message>"} to p.out.
func (p *Presenter) ShowError(err error) {
	type errJSON struct {
		Error string `json:"error"`
	}
	p.writeJSON(p.out, errJSON{Error: err.Error()})
}

// writeJSON marshals v and writes the result followed by a newline. Errors are silently discarded.
func (p *Presenter) writeJSON(w io.Writer, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "%s\n", data)
}
