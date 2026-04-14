package cli_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/cli"
	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubJDFetcher struct{}

var _ port.JDFetcher = (*stubJDFetcher)(nil)

func (s *stubJDFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return "fake job description text", nil
}

type stubLLMClient struct{}

var _ port.LLMClient = (*stubLLMClient)(nil)

func (s *stubLLMClient) ChatComplete(_ context.Context, _ []model.ChatMessage, _ model.ChatOptions) (string, error) {
	return `{"title":"SWE","company":"Acme","required":["go"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`, nil
}

type stubScorer struct{}

var _ port.Scorer = (*stubScorer)(nil)

func (s *stubScorer) Score(_ *model.ScorerInput) (model.ScoreResult, error) {
	return model.ScoreResult{
		Breakdown: model.ScoreBreakdown{
			KeywordMatch:   0.9,
			ExperienceFit:  0.9,
			ImpactEvidence: 0.9,
			ATSFormat:      0.9,
			Readability:    0.9,
		},
	}, nil
}

type stubCoverLetterGen struct{}

var _ port.CoverLetterGenerator = (*stubCoverLetterGen)(nil)

func (s *stubCoverLetterGen) Generate(_ context.Context, _ *model.CoverLetterInput) (model.CoverLetterResult, error) {
	return model.CoverLetterResult{Text: "Cover letter.", Channel: model.ChannelCold}, nil
}

type stubResumeRepo struct{}

var _ port.ResumeRepository = (*stubResumeRepo)(nil)

func (s *stubResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return []model.ResumeFile{{Label: "main", Path: "/fake/main.txt"}}, nil
}

type stubDocumentLoader struct{}

var _ port.DocumentLoader = (*stubDocumentLoader)(nil)

func (s *stubDocumentLoader) Load(_ string) (string, error) {
	return "golang experience senior engineer 5 years", nil
}

func (s *stubDocumentLoader) Supports(_ string) bool { return true }

type stubApplicationRepository struct{}

var _ port.ApplicationRepository = (*stubApplicationRepository)(nil)

func (s *stubApplicationRepository) Get(_ string) (*model.ApplicationRecord, bool, error) {
	return nil, false, nil
}
func (s *stubApplicationRepository) Put(_ *model.ApplicationRecord) error    { return nil }
func (s *stubApplicationRepository) Update(_ *model.ApplicationRecord) error { return nil }
func (s *stubApplicationRepository) List() ([]*model.ApplicationRecord, error) {
	return nil, nil
}

type stubAugmenter struct{}

var _ port.Augmenter = (*stubAugmenter)(nil)

func (s *stubAugmenter) AugmentResumeText(_ context.Context, input model.AugmentInput) (string, *model.ReferenceData, error) {
	return input.ResumeText, input.RefData, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// stubApplyConfig returns an ApplyConfig with all stubs — no filesystem/SQLite.
func stubApplyConfig() pipeline.ApplyConfig {
	return pipeline.ApplyConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       &stubLLMClient{},
		Scorer:    &stubScorer{},
		CLGen:     &stubCoverLetterGen{},
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubApplicationRepository{},
		Augment:   &stubAugmenter{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil, // overridden by handler internals
	}
}

// callToolRequest builds an mcp.CallToolRequest with the given arguments map.
func callToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	raw, _ := json.Marshal(args)
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: json.RawMessage(raw),
		},
	}
}

// extractText pulls the first TextContent text from a CallToolResult.
func extractText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("CallToolResult has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is not TextContent: %T", result.Content[0])
	}
	return tc.Text
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestGetScore_BothURLAndText_ReturnsError(t *testing.T) {
	cfg := stubApplyConfig()
	req := callToolRequest("get_score", map[string]any{
		"url":  "https://example.com/job",
		"text": "raw jd text",
	})

	result := cli.HandleGetScore(context.Background(), &req, &cfg)

	text := extractText(t, result)
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("response is not valid JSON: %v — got: %s", err, text)
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected error key in response, got: %v", resp)
	}
}

func TestGetScore_URLOnly_ReturnsResult(t *testing.T) {
	cfg := stubApplyConfig()
	req := callToolRequest("get_score", map[string]any{
		"url": "https://example.com/job",
	})

	result := cli.HandleGetScore(context.Background(), &req, &cfg)

	text := extractText(t, result)
	var pipelineResult model.PipelineResult
	if err := json.Unmarshal([]byte(text), &pipelineResult); err != nil {
		t.Fatalf("response is not valid PipelineResult JSON: %v — got: %s", err, text)
	}
}
