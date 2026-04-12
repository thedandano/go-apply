package headless_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/presenter/headless"
)

func TestPresenter_ShowResult(t *testing.T) {
	var out bytes.Buffer
	p := headless.New(&out)

	result := &model.PipelineResult{
		Status:     "ok",
		BestScore:  85.5,
		BestResume: "resume.pdf",
		Scores:     map[string]model.ScoreResult{},
		StartTime:  time.Now(),
		EndTime:    time.Now(),
	}

	if err := p.ShowResult(result); err != nil {
		t.Fatalf("ShowResult returned error: %v", err)
	}

	data := out.Bytes()
	if len(data) == 0 {
		t.Fatal("ShowResult wrote nothing")
	}

	var got model.PipelineResult
	if err := json.Unmarshal(bytes.TrimSpace(data), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, data)
	}

	if got.Status != "ok" {
		t.Errorf("status: got %q, want %q", got.Status, "ok")
	}
	if got.BestScore != 85.5 {
		t.Errorf("best_score: got %v, want 85.5", got.BestScore)
	}
	if got.BestResume != "resume.pdf" {
		t.Errorf("best_resume: got %q, want %q", got.BestResume, "resume.pdf")
	}
}

func TestPresenter_OnEvent_StepStarted(t *testing.T) {
	var out, events bytes.Buffer
	p := headless.NewWithEventWriter(&out, &events)

	p.OnEvent(model.StepStartedEvent{StepID: "fetch", Label: "Fetching JD"})

	data := events.Bytes()
	if len(data) == 0 {
		t.Fatal("OnEvent wrote nothing to events writer")
	}

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &got); err != nil {
		t.Fatalf("event output is not valid JSON: %v\noutput: %s", err, data)
	}

	if got["event"] != "step_started" {
		t.Errorf("event: got %q, want %q", got["event"], "step_started")
	}
	if got["step_id"] != "fetch" {
		t.Errorf("step_id: got %q, want %q", got["step_id"], "fetch")
	}
	if got["label"] != "Fetching JD" {
		t.Errorf("label: got %q, want %q", got["label"], "Fetching JD")
	}

	// ShowResult should write nothing to out
	if out.Len() != 0 {
		t.Errorf("OnEvent wrote %d bytes to out (expected 0)", out.Len())
	}
}

func TestPresenter_OnEvent_StepCompleted(t *testing.T) {
	var out, events bytes.Buffer
	p := headless.NewWithEventWriter(&out, &events)

	p.OnEvent(model.StepCompletedEvent{StepID: "keywords", Label: "Extracting keywords", ElapsedMS: 150})

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(events.Bytes()), &got); err != nil {
		t.Fatalf("event output is not valid JSON: %v", err)
	}
	if got["event"] != "step_completed" {
		t.Errorf("event: got %q, want %q", got["event"], "step_completed")
	}
	if got["step_id"] != "keywords" {
		t.Errorf("step_id: got %q", got["step_id"])
	}
}

func TestPresenter_OnEvent_StepFailed(t *testing.T) {
	var out, events bytes.Buffer
	p := headless.NewWithEventWriter(&out, &events)

	p.OnEvent(model.StepFailedEvent{StepID: "augment", Label: "Augmenting resume", Err: "embedding failed"})

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(events.Bytes()), &got); err != nil {
		t.Fatalf("event output is not valid JSON: %v", err)
	}
	if got["event"] != "step_failed" {
		t.Errorf("event: got %q, want %q", got["event"], "step_failed")
	}
	if got["error"] != "embedding failed" {
		t.Errorf("error: got %q, want %q", got["error"], "embedding failed")
	}
}

func TestPresenter_OnEvent_Unknown_NoPanic(t *testing.T) {
	var out, events bytes.Buffer
	p := headless.NewWithEventWriter(&out, &events)

	// Should not panic or write anything
	p.OnEvent("unknown event type")
	p.OnEvent(42)
	p.OnEvent(nil)

	if events.Len() != 0 {
		t.Errorf("expected no output for unknown event, got %d bytes", events.Len())
	}
}

func TestPresenter_ShowError(t *testing.T) {
	var out, events bytes.Buffer
	p := headless.NewWithEventWriter(&out, &events)

	p.ShowError(errors.New("something went wrong"))

	// errors go to stderr (events writer), not stdout
	data := events.Bytes()
	if len(data) == 0 {
		t.Fatal("ShowError wrote nothing to stderr")
	}

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(data), &got); err != nil {
		t.Fatalf("ShowError output is not valid JSON: %v\noutput: %s", err, data)
	}

	if got["error"] != "something went wrong" {
		t.Errorf("error: got %q, want %q", got["error"], "something went wrong")
	}

	// stdout must remain empty — errors must not pollute the JSON result stream
	if out.Len() != 0 {
		t.Errorf("ShowError wrote %d bytes to stdout (expected 0)", out.Len())
	}
}

func TestPresenter_ShowTailorResult(t *testing.T) {
	var out bytes.Buffer
	p := headless.New(&out)

	result := &model.TailorResult{
		ResumeLabel: "resume.pdf",
		TierApplied: model.TierKeyword,
		OutputPath:  "/tmp/tailored.pdf",
	}

	if err := p.ShowTailorResult(result); err != nil {
		t.Fatalf("ShowTailorResult returned error: %v", err)
	}

	var got model.TailorResult
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.ResumeLabel != "resume.pdf" {
		t.Errorf("resume_label: got %q, want %q", got.ResumeLabel, "resume.pdf")
	}
}
