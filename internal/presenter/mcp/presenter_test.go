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

	// Fields must remain nil — no side effects.
	if p.Result != nil {
		t.Error("Result should remain nil after OnEvent calls")
	}
	if p.TailorResult != nil {
		t.Error("TailorResult should remain nil after OnEvent calls")
	}
}

func TestCapturingPresenter_ShowResult_Stores(t *testing.T) {
	p := mcppres.New()
	result := &model.PipelineResult{
		Status:    "success",
		BestScore: 0.85,
	}

	err := p.ShowResult(result)

	if err != nil {
		t.Errorf("ShowResult returned unexpected error: %v", err)
	}
	if p.Result != result {
		t.Errorf("Result = %v, want %v", p.Result, result)
	}
}

func TestCapturingPresenter_ShowTailorResult_Stores(t *testing.T) {
	p := mcppres.New()
	tailorResult := &model.TailorResult{
		ResumeLabel: "my-resume",
		OutputPath:  "/tmp/output.docx",
	}

	err := p.ShowTailorResult(tailorResult)

	if err != nil {
		t.Errorf("ShowTailorResult returned unexpected error: %v", err)
	}
	if p.TailorResult != tailorResult {
		t.Errorf("TailorResult = %v, want %v", p.TailorResult, tailorResult)
	}
}

func TestCapturingPresenter_ShowTailorResult_IncludesChangelog(t *testing.T) {
	p := mcppres.New()
	tailorResult := &model.TailorResult{
		ResumeLabel: "main",
		Changelog: []model.ChangelogEntry{
			{Kind: model.ChangelogSkillAdd, Tier: model.ChangelogTier1, Keyword: "kubernetes"},
		},
	}

	if err := p.ShowTailorResult(tailorResult); err != nil {
		t.Fatalf("ShowTailorResult: %v", err)
	}
	if len(p.TailorResult.Changelog) != 1 {
		t.Fatalf("Changelog len = %d, want 1", len(p.TailorResult.Changelog))
	}
	if p.TailorResult.Changelog[0].Kind != model.ChangelogSkillAdd {
		t.Errorf("Kind = %v, want ChangelogSkillAdd", p.TailorResult.Changelog[0].Kind)
	}
	if p.TailorResult.Changelog[0].Keyword != "kubernetes" {
		t.Errorf("Keyword = %q, want kubernetes", p.TailorResult.Changelog[0].Keyword)
	}
}
