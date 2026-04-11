package coverletter_test

import (
	"context"
	"errors"
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
	d := config.EmbeddedDefaults()
	return d
}

func TestGenerate_ReturnsResult(t *testing.T) {
	stub := &stubLLMClient{
		response: "Dear Hiring Manager, I am excited to apply for this role. My skills align well with your requirements. Thank you for your consideration.",
	}

	gen := coverletter.New(stub, testDefaults())

	input := port.CoverLetterInput{
		JD: model.JDData{
			Title:    "Software Engineer",
			Company:  "Acme Corp",
			Location: "Remote",
		},
		Scores: map[string]model.ScoreResult{
			"resume_a": {
				ResumeLabel: "resume_a",
				Breakdown:   model.ScoreBreakdown{KeywordMatch: 30, ExperienceFit: 20, ImpactEvidence: 8, ATSFormat: 8, Readability: 8},
				Keywords: model.KeywordResult{
					ReqMatched: []string{"Go", "REST"},
				},
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

func TestGenerate_UsesHighestScoringResume(t *testing.T) {
	stub := &stubLLMClient{
		response: "Cover letter text.",
	}

	gen := coverletter.New(stub, testDefaults())

	input := port.CoverLetterInput{
		JD: model.JDData{
			Title:   "Data Engineer",
			Company: "DataCo",
		},
		Scores: map[string]model.ScoreResult{
			"low_score": {
				ResumeLabel: "low_score",
				Breakdown:   model.ScoreBreakdown{KeywordMatch: 10, ExperienceFit: 10, ImpactEvidence: 5, ATSFormat: 5, Readability: 5},
				Keywords: model.KeywordResult{
					ReqMatched: []string{"Python"},
				},
			},
			"high_score": {
				ResumeLabel: "high_score",
				Breakdown:   model.ScoreBreakdown{KeywordMatch: 40, ExperienceFit: 22, ImpactEvidence: 9, ATSFormat: 9, Readability: 9},
				Keywords: model.KeywordResult{
					ReqMatched: []string{"Spark", "Kafka", "SQL"},
				},
			},
		},
		Channel: model.ChannelReferral,
		Profile: model.UserProfile{
			Name: "John Smith",
		},
	}

	_, err := gen.Generate(context.Background(), &input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(stub.recorded) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %d", len(stub.recorded))
	}

	userMsg := stub.recorded[len(stub.recorded)-1].Content
	for _, kw := range []string{"Spark", "Kafka", "SQL"} {
		if !strings.Contains(userMsg, kw) {
			t.Errorf("expected prompt to contain keyword %q from highest scoring resume", kw)
		}
	}
	// low_score resume's unique keyword should NOT dominate the prompt
	// (Python might appear if it's a preferred keyword; just verify high-score keywords are present)
}

func TestGenerate_LLMError(t *testing.T) {
	stub := &stubLLMClient{
		err: errors.New("connection refused"),
	}

	gen := coverletter.New(stub, testDefaults())

	input := port.CoverLetterInput{
		JD: model.JDData{Title: "Engineer", Company: "Corp"},
		Scores: map[string]model.ScoreResult{
			"r": {ResumeLabel: "r"},
		},
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
