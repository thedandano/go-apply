package coverletter_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/coverletter"
)

// Compile-time interface satisfaction check for stub.
var _ port.LLMClient = (*stubLLMClient)(nil)

type stubLLMClient struct {
	response string
	err      error
	recorded []port.ChatMessage
}

func (s *stubLLMClient) ChatComplete(_ context.Context, messages []port.ChatMessage, _ port.ChatOptions) (string, error) {
	s.recorded = messages
	return s.response, s.err
}

func testDefaults() *config.AppDefaults {
	return config.EmbeddedDefaults()
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGenerate_ReturnsResult(t *testing.T) {
	stub := &stubLLMClient{
		response: "Dear Hiring Manager, I am excited to apply for this role. My skills align well with your requirements. Thank you for your consideration.",
	}

	gen := coverletter.New(stub, testDefaults(), discardLogger())

	input := port.CoverLetterInput{
		JD: model.JDData{
			Title:    "Software Engineer",
			Company:  "Acme Corp",
			Location: "Remote",
			Required: []string{"Go", "REST"},
		},
		Scores: map[string]model.ScoreResult{
			"resume_a": {
				ResumeLabel: "resume_a",
				Breakdown:   model.ScoreBreakdown{KeywordMatch: 30, ExperienceFit: 20, ImpactEvidence: 8, ATSFormat: 8, Readability: 8},
				Keywords:    model.KeywordResult{ReqMatched: []string{"Go", "REST"}},
			},
		},
		Channel: model.ChannelCold,
		Profile: model.UserProfile{
			Name:              "Jane Doe",
			Occupation:        "Software Engineer",
			Location:          "San Francisco, CA",
			YearsOfExperience: 5,
		},
	}

	result, err := gen.Generate(context.Background(), &input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Text == "" {
		t.Error("expected non-empty Text")
	}
	if result.Channel != model.ChannelCold {
		t.Errorf("expected channel COLD, got %s", result.Channel)
	}
	if result.WordCount == 0 {
		t.Error("expected WordCount > 0")
	}
	if result.SentenceCount == 0 {
		t.Error("expected SentenceCount > 0")
	}
}

func TestGenerate_PromptIncludesJDSkills(t *testing.T) {
	stub := &stubLLMClient{response: "Cover letter."}
	gen := coverletter.New(stub, testDefaults(), discardLogger())

	input := port.CoverLetterInput{
		JD: model.JDData{
			Title:     "Backend Engineer",
			Company:   "TechCo",
			Required:  []string{"Kubernetes", "PostgreSQL"},
			Preferred: []string{"Rust"},
		},
		Scores:  map[string]model.ScoreResult{"r": {ResumeLabel: "r"}},
		Channel: model.ChannelCold,
		Profile: model.UserProfile{Name: "Test"},
	}

	_, err := gen.Generate(context.Background(), &input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userMsg := stub.recorded[len(stub.recorded)-1].Content
	for _, skill := range []string{"Kubernetes", "PostgreSQL", "Rust"} {
		if !strings.Contains(userMsg, skill) {
			t.Errorf("prompt missing JD skill %q", skill)
		}
	}
}

func TestGenerate_PromptIncludesWordTargets(t *testing.T) {
	stub := &stubLLMClient{response: "Cover letter."}
	gen := coverletter.New(stub, testDefaults(), discardLogger())

	input := port.CoverLetterInput{
		JD:      model.JDData{Title: "Eng", Company: "Co"},
		Scores:  map[string]model.ScoreResult{"r": {ResumeLabel: "r"}},
		Channel: model.ChannelCold,
		Profile: model.UserProfile{Name: "Test"},
	}

	_, err := gen.Generate(context.Background(), &input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userMsg := stub.recorded[len(stub.recorded)-1].Content
	// Prompt must communicate word/sentence constraints derived from defaults.
	if !strings.Contains(userMsg, "words") {
		t.Error("prompt does not mention word count target from defaults")
	}
	if !strings.Contains(userMsg, "sentences") {
		t.Error("prompt does not mention sentence count target from defaults")
	}
}

func TestGenerate_UsesHighestScoringResume(t *testing.T) {
	stub := &stubLLMClient{response: "Cover letter text."}
	gen := coverletter.New(stub, testDefaults(), discardLogger())

	input := port.CoverLetterInput{
		JD:      model.JDData{Title: "Data Engineer", Company: "DataCo"},
		Channel: model.ChannelReferral,
		Profile: model.UserProfile{Name: "John Smith"},
		Scores: map[string]model.ScoreResult{
			"low_score": {
				ResumeLabel: "low_score",
				Breakdown:   model.ScoreBreakdown{KeywordMatch: 10, ExperienceFit: 10, ImpactEvidence: 5, ATSFormat: 5, Readability: 5},
				Keywords:    model.KeywordResult{ReqMatched: []string{"Python"}},
			},
			"high_score": {
				ResumeLabel: "high_score",
				Breakdown:   model.ScoreBreakdown{KeywordMatch: 40, ExperienceFit: 22, ImpactEvidence: 9, ATSFormat: 9, Readability: 9},
				Keywords:    model.KeywordResult{ReqMatched: []string{"Spark", "Kafka", "SQL"}},
			},
		},
	}

	_, err := gen.Generate(context.Background(), &input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	userMsg := stub.recorded[len(stub.recorded)-1].Content
	// High-score keywords must appear in prompt.
	for _, kw := range []string{"Spark", "Kafka", "SQL"} {
		if !strings.Contains(userMsg, kw) {
			t.Errorf("prompt missing high-score keyword %q", kw)
		}
	}
	// Low-score unique keyword must NOT appear (it's not in the JD Required/Preferred either).
	if strings.Contains(userMsg, "Python") {
		t.Error("prompt contains low-score resume's unique keyword 'Python' — wrong resume selected")
	}
}

func TestGenerate_EmptyScores(t *testing.T) {
	stub := &stubLLMClient{response: "Cover letter with no score context."}
	gen := coverletter.New(stub, testDefaults(), discardLogger())

	input := port.CoverLetterInput{
		JD:      model.JDData{Title: "Engineer", Company: "Corp"},
		Scores:  map[string]model.ScoreResult{},
		Channel: model.ChannelCold,
		Profile: model.UserProfile{Name: "Test User"},
	}

	// Empty scores is a degraded-mode path — should succeed, not panic.
	result, err := gen.Generate(context.Background(), &input)
	if err != nil {
		t.Fatalf("expected no error for empty scores, got: %v", err)
	}
	if result.Text == "" {
		t.Error("expected non-empty Text even with empty scores")
	}
}

func TestGenerate_WarnWhenJDRawTextMissing(t *testing.T) {
	var warnFired bool
	handler := &capturingHandler{fn: func(r slog.Record) {
		if r.Level == slog.LevelWarn && strings.Contains(r.Message, "JDRawText") {
			warnFired = true
		}
	}}
	log := slog.New(handler)

	stub := &stubLLMClient{response: "Cover letter."}
	gen := coverletter.New(stub, testDefaults(), log)

	input := port.CoverLetterInput{
		JD:      model.JDData{Title: "SWE", Company: "Acme"},
		Scores:  map[string]model.ScoreResult{"r": {ResumeLabel: "r"}},
		Channel: model.ChannelCold,
		Profile: model.UserProfile{Name: "Test"},
		// JDRawText intentionally omitted
	}
	if _, err := gen.Generate(context.Background(), &input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !warnFired {
		t.Error("expected Warn log for missing JDRawText, none recorded")
	}
}

// capturingHandler is a minimal slog.Handler that calls fn for each record.
type capturingHandler struct{ fn func(slog.Record) }

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // hugeParam: slog.Handler interface requires slog.Record by value
	h.fn(r)
	return nil
}
func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func TestGenerate_PromptIncludesJDRawText(t *testing.T) {
	stub := &stubLLMClient{response: "Cover letter."}
	gen := coverletter.New(stub, testDefaults(), discardLogger())

	input := port.CoverLetterInput{
		JD:        model.JDData{Title: "SRE", Company: "CloudCo"},
		JDRawText: "We are looking for a site reliability engineer with expertise in Kubernetes and Terraform.",
		Scores:    map[string]model.ScoreResult{"r": {ResumeLabel: "r"}},
		Channel:   model.ChannelCold,
		Profile:   model.UserProfile{Name: "Test"},
	}

	_, err := gen.Generate(context.Background(), &input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userMsg := stub.recorded[len(stub.recorded)-1].Content
	if !strings.Contains(userMsg, input.JDRawText) {
		t.Error("prompt does not contain JDRawText — LLM cannot tailor to job-specific details")
	}
}

func TestGenerate_EmptyLLMResponse(t *testing.T) {
	stub := &stubLLMClient{response: ""}
	gen := coverletter.New(stub, testDefaults(), discardLogger())

	input := port.CoverLetterInput{
		JD:      model.JDData{Title: "Engineer", Company: "Corp"},
		Scores:  map[string]model.ScoreResult{"r": {ResumeLabel: "r"}},
		Channel: model.ChannelCold,
		Profile: model.UserProfile{Name: "Test"},
	}

	_, err := gen.Generate(context.Background(), &input)
	if err == nil {
		t.Fatal("expected error for empty LLM response, got nil")
	}
	if !strings.Contains(err.Error(), "empty response") {
		t.Errorf("error %q should mention 'empty response'", err.Error())
	}
}

func TestGenerate_LLMError(t *testing.T) {
	stub := &stubLLMClient{err: errors.New("connection refused")}
	gen := coverletter.New(stub, testDefaults(), discardLogger())

	input := port.CoverLetterInput{
		JD:      model.JDData{Title: "Engineer", Company: "Corp"},
		Scores:  map[string]model.ScoreResult{"r": {ResumeLabel: "r"}},
		Channel: model.ChannelRecruiter,
		Profile: model.UserProfile{Name: "Test User"},
	}

	_, err := gen.Generate(context.Background(), &input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected underlying error in message, got: %v", err)
	}
}
