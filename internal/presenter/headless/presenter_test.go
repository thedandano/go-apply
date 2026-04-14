package headless_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/presenter/headless"
)

func TestJSONPresenter_OnEvent_WritesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := headless.NewWith(&stdout, &stderr)

	p.OnEvent(model.StepStartedEvent{StepID: "fetch", Label: "Fetching JD"})

	if stdout.Len() != 0 {
		t.Errorf("OnEvent wrote to stdout; want stderr only")
	}
	if stderr.Len() == 0 {
		t.Error("OnEvent wrote nothing to stderr")
	}
	var event model.StepStartedEvent
	if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &event); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nOutput: %s", err, stderr.String())
	}
	if event.StepID != "fetch" {
		t.Errorf("step_id = %q, want fetch", event.StepID)
	}
}

func TestJSONPresenter_ShowResult_WritesToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := headless.NewWith(&stdout, &stderr)

	result := model.NewPipelineResult()
	result.Status = "success"
	result.BestResume = "resume.pdf"
	result.BestScore = 0.85

	if err := p.ShowResult(result); err != nil {
		t.Fatalf("ShowResult returned error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("ShowResult wrote to stderr; want stdout only")
	}
	if stdout.Len() == 0 {
		t.Error("ShowResult wrote nothing to stdout")
	}
	var got model.PipelineResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nOutput: %s", err, stdout.String())
	}
	if got.Status != "success" {
		t.Errorf("status = %q, want success", got.Status)
	}
	if got.BestScore != 0.85 {
		t.Errorf("best_score = %v, want 0.85", got.BestScore)
	}
}

func TestJSONPresenter_ShowTailorResult_WritesToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := headless.NewWith(&stdout, &stderr)

	result := &model.TailorResult{ResumeLabel: "resume", TierApplied: model.TierKeyword}

	if err := p.ShowTailorResult(result); err != nil {
		t.Fatalf("ShowTailorResult returned error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("ShowTailorResult wrote to stderr; want stdout only")
	}
	var got model.TailorResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nOutput: %s", err, stdout.String())
	}
	if got.ResumeLabel != "resume" {
		t.Errorf("resume_label = %q, want resume", got.ResumeLabel)
	}
}

func TestJSONPresenter_ShowError_WritesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := headless.NewWith(&stdout, &stderr)

	p.ShowError(errors.New("something went wrong"))

	if stdout.Len() != 0 {
		t.Errorf("ShowError wrote to stdout; want stderr only")
	}
	if stderr.Len() == 0 {
		t.Error("ShowError wrote nothing to stderr")
	}
	var got map[string]string
	if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &got); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nOutput: %s", err, stderr.String())
	}
	if !strings.Contains(got["error"], "something went wrong") {
		t.Errorf("error message = %q, want to contain 'something went wrong'", got["error"])
	}
}
