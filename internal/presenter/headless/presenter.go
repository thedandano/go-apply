// Package headless provides a JSON-based presenter that writes pipeline results
// to stdout and events/errors to stderr. Used by the --headless CLI flag and
// MCP server contexts where no terminal UI is available.
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
var _ port.Presenter = (*JSONPresenter)(nil)

// JSONPresenter implements port.Presenter by writing JSON to stdout/stderr.
// Events and errors go to stderr; results go to stdout.
type JSONPresenter struct {
	stdout io.Writer
	stderr io.Writer
}

// New constructs a JSONPresenter writing to os.Stdout and os.Stderr.
func New() *JSONPresenter {
	return &JSONPresenter{stdout: os.Stdout, stderr: os.Stderr}
}

// NewWith constructs a JSONPresenter with explicit writers — for testing.
func NewWith(stdout, stderr io.Writer) *JSONPresenter {
	return &JSONPresenter{stdout: stdout, stderr: stderr}
}

// OnEvent writes a JSON-encoded event to stderr.
// Events are: model.StepStartedEvent, model.StepCompletedEvent, model.StepFailedEvent.
func (p *JSONPresenter) OnEvent(event any) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(p.stderr, "%s\n", data)
}

// ShowResult writes the full pipeline result as indented JSON to stdout.
func (p *JSONPresenter) ShowResult(result *model.PipelineResult) error {
	enc := json.NewEncoder(p.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// ShowTailorResult writes a tailor result as indented JSON to stdout.
func (p *JSONPresenter) ShowTailorResult(result *model.TailorResult) error {
	enc := json.NewEncoder(p.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// ShowError writes a JSON error object to stderr.
func (p *JSONPresenter) ShowError(err error) {
	_ = json.NewEncoder(p.stderr).Encode(map[string]string{"error": err.Error()}) // #nosec G104 -- best-effort error reporting to stderr; encoding a string map cannot fail
}
