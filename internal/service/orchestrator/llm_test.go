package orchestrator_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/orchestrator"
)

// stubLLMClient is a hand-rolled mock for port.LLMClient.
type stubLLMClient struct {
	response string
	err      error
}

var _ port.LLMClient = (*stubLLMClient)(nil)

func (s *stubLLMClient) ChatComplete(_ context.Context, _ []model.ChatMessage, _ model.ChatOptions) (string, error) {
	return s.response, s.err
}

func TestLLMOrchestrator_ExtractKeywords_HappyPath(t *testing.T) {
	payload := `{"title":"Senior Engineer","company":"Acme","required":["golang","kubernetes"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":5}`
	client := &stubLLMClient{response: payload}
	orch := orchestrator.NewLLMOrchestrator(client)

	jd, err := orch.ExtractKeywords(context.Background(), port.ExtractKeywordsInput{
		JDText: "We are looking for a Senior Go engineer...",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jd.Title != "Senior Engineer" {
		t.Errorf("Title = %q, want %q", jd.Title, "Senior Engineer")
	}
	if jd.Company != "Acme" {
		t.Errorf("Company = %q, want %q", jd.Company, "Acme")
	}
	if len(jd.Required) != 2 {
		t.Errorf("Required len = %d, want 2", len(jd.Required))
	}
	if jd.Seniority != model.SenioritySenior {
		t.Errorf("Seniority = %q, want %q", jd.Seniority, model.SenioritySenior)
	}
	if jd.RequiredYears != 5 {
		t.Errorf("RequiredYears = %v, want 5", jd.RequiredYears)
	}
}

func TestLLMOrchestrator_ExtractKeywords_InvalidJSON(t *testing.T) {
	client := &stubLLMClient{response: "not valid json at all"}
	orch := orchestrator.NewLLMOrchestrator(client)

	_, err := orch.ExtractKeywords(context.Background(), port.ExtractKeywordsInput{
		JDText: "some job description",
	})
	if err == nil {
		t.Fatal("expected error on invalid JSON, got nil")
	}
}

func TestLLMOrchestrator_ExtractKeywords_LLMError(t *testing.T) {
	client := &stubLLMClient{err: fmt.Errorf("connection refused")}
	orch := orchestrator.NewLLMOrchestrator(client)

	_, err := orch.ExtractKeywords(context.Background(), port.ExtractKeywordsInput{
		JDText: "some job description",
	})
	if err == nil {
		t.Fatal("expected error on LLM failure, got nil")
	}
}

func TestLLMOrchestrator_ExtractKeywords_EmptyInput(t *testing.T) {
	client := &stubLLMClient{response: "{}"}
	orch := orchestrator.NewLLMOrchestrator(client)

	_, err := orch.ExtractKeywords(context.Background(), port.ExtractKeywordsInput{
		JDText: "",
	})
	if err == nil {
		t.Fatal("expected error on empty JD text, got nil")
	}
}

func TestLLMOrchestrator_PlanT1_HappyPath(t *testing.T) {
	payload := `{"skill_adds":["rust","terraform"]}`
	client := &stubLLMClient{response: payload}
	orch := orchestrator.NewLLMOrchestrator(client)

	out, err := orch.PlanT1(context.Background(), &port.PlanT1Input{
		JDData:     model.JDData{Required: []string{"rust", "terraform"}},
		ResumeText: "experience in Go",
		SkillsRef:  "Skills: Go, Python",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.SkillAdds) != 2 {
		t.Errorf("SkillAdds len = %d, want 2", len(out.SkillAdds))
	}
}

func TestLLMOrchestrator_PlanT2_HappyPath(t *testing.T) {
	payload := `{"rewrites":[{"original":"built systems","replacement":"built distributed systems handling 10k rps"}]}`
	client := &stubLLMClient{response: payload}
	orch := orchestrator.NewLLMOrchestrator(client)

	out, err := orch.PlanT2(context.Background(), &port.PlanT2Input{
		JDData:          model.JDData{Required: []string{"distributed systems"}},
		ResumeText:      "built systems",
		Accomplishments: "led migration to distributed system",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Rewrites) != 1 {
		t.Errorf("Rewrites len = %d, want 1", len(out.Rewrites))
	}
	if out.Rewrites[0].Original != "built systems" {
		t.Errorf("Rewrites[0].Original = %q, want %q", out.Rewrites[0].Original, "built systems")
	}
}

func TestLLMOrchestrator_GenerateCoverLetter_HappyPath(t *testing.T) {
	expected := "Dear Hiring Manager, I am excited to apply..."
	client := &stubLLMClient{response: expected}
	orch := orchestrator.NewLLMOrchestrator(client)

	out, err := orch.GenerateCoverLetter(context.Background(), &port.CoverLetterInput{
		JDData:        model.JDData{Title: "Engineer", Company: "Acme"},
		ResumeText:    "resume text",
		CandidateName: "Alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != expected {
		t.Errorf("cover letter = %q, want %q", out, expected)
	}
}
