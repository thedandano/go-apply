package mcp_test

import (
	"errors"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	mcppres "github.com/thedandano/go-apply/internal/presenter/mcp"
)

func TestMCPPresenter_CapturesResult(t *testing.T) {
	p := mcppres.New()

	result := &model.PipelineResult{
		Status:     "ok",
		BestScore:  92.0,
		BestResume: "resume.pdf",
		Scores:     map[string]model.ScoreResult{},
	}

	if err := p.ShowResult(result); err != nil {
		t.Fatalf("ShowResult returned error: %v", err)
	}

	got := p.Result()
	if got == nil {
		t.Fatal("Result() returned nil after ShowResult")
	}
	if got.Status != "ok" {
		t.Errorf("status: got %q, want %q", got.Status, "ok")
	}
	if got.BestScore != 92.0 {
		t.Errorf("best_score: got %v, want 92.0", got.BestScore)
	}
	if got.BestResume != "resume.pdf" {
		t.Errorf("best_resume: got %q, want %q", got.BestResume, "resume.pdf")
	}
}

func TestMCPPresenter_CapturesError(t *testing.T) {
	p := mcppres.New()

	p.ShowError(errors.New("pipeline failed"))

	if p.Err() == nil {
		t.Fatal("Err() returned nil after ShowError")
	}
	if p.Err().Error() != "pipeline failed" {
		t.Errorf("Err(): got %q, want %q", p.Err().Error(), "pipeline failed")
	}
}

func TestMCPPresenter_OnEvent_Ignores_Unknown(_ *testing.T) {
	p := mcppres.New()

	// Should not panic with unknown event types
	p.OnEvent("unknown string event")
	p.OnEvent(42)
	p.OnEvent(nil)
	p.OnEvent(struct{ Foo string }{"bar"})
}
