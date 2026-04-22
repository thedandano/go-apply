package mcpserver

// session_tools_retention_test.go — internal package tests for T1 session retention.
// Must be package mcpserver (not mcpserver_test) to access unexported identifiers.

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// retentionScorerFailAfter succeeds on calls 1..n then fails.
type retentionScorerFailAfter struct {
	callsLeft atomic.Int32
}

var _ port.Scorer = (*retentionScorerFailAfter)(nil)

func newRetentionScorerFailAfter(n int) *retentionScorerFailAfter {
	s := &retentionScorerFailAfter{}
	s.callsLeft.Store(int32(n)) //nolint:gosec // small test constant
	return s
}

func (s *retentionScorerFailAfter) Score(_ *model.ScorerInput) (model.ScoreResult, error) {
	if s.callsLeft.Add(-1) >= 0 {
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
	return model.ScoreResult{}, errors.New("rescore: injected failure")
}

type retentionJDFetcher struct{}

func (r *retentionJDFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return "fake job description text", nil
}

type retentionLLMClient struct{}

func (r *retentionLLMClient) ChatComplete(_ context.Context, _ []model.ChatMessage, _ model.ChatOptions) (string, error) {
	return `{"title":"SWE","company":"Acme","required":["go"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`, nil
}

type retentionResumeRepo struct{}

func (r *retentionResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return []model.ResumeFile{{Label: "main", Path: "/fake/main.txt"}}, nil
}

type retentionDocumentLoader struct{}

func (r *retentionDocumentLoader) Load(_ string) (string, error) {
	return "golang experience senior engineer 5 years", nil
}
func (r *retentionDocumentLoader) Supports(_ string) bool { return true }

type retentionAppRepo struct{}

func (r *retentionAppRepo) Get(_ string) (*model.ApplicationRecord, bool, error) {
	return nil, false, nil
}
func (r *retentionAppRepo) Put(_ *model.ApplicationRecord) error      { return nil }
func (r *retentionAppRepo) Update(_ *model.ApplicationRecord) error   { return nil }
func (r *retentionAppRepo) List() ([]*model.ApplicationRecord, error) { return nil, nil }

func callToolRequestInternal(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

// TestHandleSubmitTailorT1_RescoreFailure_SessionRetained verifies that when tailor
// succeeds and the subsequent rescore fails, the session retains TailoredText and
// its State is stateT1Applied.
func TestHandleSubmitTailorT1_RescoreFailure_SessionRetained(t *testing.T) {
	// 1 successful score during submit_keywords (one resume), then fail on rescore.
	failingScorer := newRetentionScorerFailAfter(1)
	cfg := pipeline.ApplyConfig{
		Fetcher:   &retentionJDFetcher{},
		LLM:       &retentionLLMClient{},
		Scorer:    failingScorer,
		CLGen:     nil,
		Resumes:   &retentionResumeRepo{},
		Loader:    &retentionDocumentLoader{},
		AppRepo:   &retentionAppRepo{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}

	// load_jd
	loadReq := callToolRequestInternal("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadResult := HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg)
	loadText := extractTextInternal(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	// submit_keywords — consumes the 1 allowed score call
	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequestInternal("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	// submit_tailor_t1 — rescore fails, but session state must still be updated
	t1Req := callToolRequestInternal("submit_tailor_t1", map[string]any{
		"session_id": sessionID,
		"skill_adds": `["Terraform"]`,
	})
	result := HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfg, &config.Config{})
	text := extractTextInternal(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Fatalf("status = %v, want error — full: %s", env["status"], text)
	}

	// Verify session state was retained despite rescore failure.
	sess := sessions.Get(sessionID)
	if sess == nil {
		t.Fatal("session not found after T1")
	}
	if sess.TailoredText == "" {
		t.Error("sess.TailoredText is empty; expected T1 output to be retained on rescore failure")
	}
	if sess.State != stateT1Applied {
		t.Errorf("sess.State = %v, want stateT1Applied", sess.State)
	}
}

// TestHandleSubmitTailorT2_RescoreFailure_SessionRetained verifies that when tailor
// succeeds and the subsequent rescore fails, the session retains TailoredText and
// its State is stateT2Applied.
func TestHandleSubmitTailorT2_RescoreFailure_SessionRetained(t *testing.T) {
	// 1 successful score during submit_keywords (one resume), then fail on rescore.
	failingScorer := newRetentionScorerFailAfter(1)
	cfg := pipeline.ApplyConfig{
		Fetcher:   &retentionJDFetcher{},
		LLM:       &retentionLLMClient{},
		Scorer:    failingScorer,
		CLGen:     nil,
		Resumes:   &retentionResumeRepo{},
		Loader:    &retentionDocumentLoader{},
		AppRepo:   &retentionAppRepo{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}

	// load_jd
	loadReq := callToolRequestInternal("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadResult := HandleLoadJDWithConfig(context.Background(), &loadReq, &cfg)
	loadText := extractTextInternal(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	// submit_keywords — consumes the 1 allowed score call
	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequestInternal("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &cfg, &config.Config{})

	// submit_tailor_t2 — rescore fails, but session state must still be updated
	t2Req := callToolRequestInternal("submit_tailor_t2", map[string]any{
		"session_id":      sessionID,
		"bullet_rewrites": `[{"original":"golang experience senior engineer 5 years","replacement":"golang experience senior engineer 5 years, Kubernetes"}]`,
	})
	result := HandleSubmitTailorT2WithConfig(context.Background(), &t2Req, &cfg, &config.Config{})
	text := extractTextInternal(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["status"] != "error" {
		t.Fatalf("status = %v, want error — full: %s", env["status"], text)
	}

	// Verify session state was retained despite rescore failure.
	sess := sessions.Get(sessionID)
	if sess == nil {
		t.Fatal("session not found after T2")
	}
	if sess.TailoredText == "" {
		t.Error("sess.TailoredText is empty; expected T2 output to be retained on rescore failure")
	}
	if sess.State != stateT2Applied {
		t.Errorf("sess.State = %v, want stateT2Applied", sess.State)
	}
}

// extractTextInternal is a package-level copy of extractText for internal tests.
func extractTextInternal(t *testing.T, result *mcp.CallToolResult) string {
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
