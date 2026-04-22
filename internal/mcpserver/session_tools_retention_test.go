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

// retentionPassingScorer always succeeds — used when both initial score and rescore must pass.
type retentionPassingScorer struct{}

var _ port.Scorer = (*retentionPassingScorer)(nil)

func (r *retentionPassingScorer) Score(_ *model.ScorerInput) (model.ScoreResult, error) {
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

// retentionPassingCfg builds an ApplyConfig where every call to Score succeeds.
func retentionPassingCfg() pipeline.ApplyConfig {
	return pipeline.ApplyConfig{
		Fetcher:   &retentionJDFetcher{},
		LLM:       &retentionLLMClient{},
		Scorer:    &retentionPassingScorer{},
		CLGen:     nil,
		Resumes:   &retentionResumeRepo{},
		Loader:    &retentionDocumentLoader{},
		AppRepo:   &retentionAppRepo{},
		Defaults:  &config.AppDefaults{},
		Presenter: nil,
	}
}

// retentionFullSetup runs load_jd → submit_keywords for the given cfg and returns the
// session ID. Fails the test immediately if any step does not return status "ok".
func retentionFullSetup(t *testing.T, cfg *pipeline.ApplyConfig) string {
	t.Helper()

	loadReq := callToolRequestInternal("load_jd", map[string]any{"jd_raw_text": "Senior Go engineer."})
	loadResult := HandleLoadJDWithConfig(context.Background(), &loadReq, cfg)
	loadText := extractTextInternal(t, loadResult)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd not JSON: %v", err)
	}
	if loadEnv["status"] != "ok" {
		t.Fatalf("load_jd status = %v, want ok — full: %s", loadEnv["status"], loadText)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go"],"preferred":[],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequestInternal("submit_keywords", map[string]any{"session_id": sessionID, "jd_json": jdJSON})
	kwResult := HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, cfg, &config.Config{})
	kwText := extractTextInternal(t, kwResult)
	var kwEnv map[string]any
	if err := json.Unmarshal([]byte(kwText), &kwEnv); err != nil {
		t.Fatalf("submit_keywords not JSON: %v", err)
	}
	if kwEnv["status"] != "ok" {
		t.Fatalf("submit_keywords status = %v, want ok — full: %s", kwEnv["status"], kwText)
	}

	return sessionID
}

// TestHandleSubmitTailor_ConcurrentSessionsNoRace proves that concurrent T1 on session A
// and T2 on session B do not produce a data race under -race.
//
// Both goroutines touch the shared sessions map (for Get calls) and their own Session
// fields (TailoredText, State) concurrently. If the mutex coverage is incomplete the
// race detector will flag it.
func TestHandleSubmitTailor_ConcurrentSessionsNoRace(t *testing.T) {
	cfgA := retentionPassingCfg()
	cfgB := retentionPassingCfg()

	// Bring both sessions to stateScored before forking goroutines.
	sessIDA := retentionFullSetup(t, &cfgA)
	sessIDB := retentionFullSetup(t, &cfgB)

	// Verify preconditions.
	for _, id := range []string{sessIDA, sessIDB} {
		s := sessions.Get(id)
		if s == nil {
			t.Fatalf("session %s not found after setup", id)
		}
		if s.State != stateScored {
			t.Fatalf("session %s state = %v, want stateScored", id, s.State)
		}
	}

	type result struct {
		text string
		err  error
	}
	resA := make(chan result, 1)
	resB := make(chan result, 1)

	// Goroutine 1: T1 on session A.
	go func() {
		t1Req := callToolRequestInternal("submit_tailor_t1", map[string]any{
			"session_id": sessIDA,
			"skill_adds": `["Terraform","gRPC"]`,
		})
		r := HandleSubmitTailorT1WithConfig(context.Background(), &t1Req, &cfgA, &config.Config{})
		if len(r.Content) == 0 {
			resA <- result{err: errors.New("T1: empty content")}
			return
		}
		tc, ok := r.Content[0].(mcp.TextContent)
		if !ok {
			resA <- result{err: errors.New("T1: content[0] not TextContent")}
			return
		}
		resA <- result{text: tc.Text}
	}()

	// Goroutine 2: T2 on session B.
	go func() {
		t2Req := callToolRequestInternal("submit_tailor_t2", map[string]any{
			"session_id":      sessIDB,
			"bullet_rewrites": `[{"original":"golang experience senior engineer 5 years","replacement":"golang experience senior engineer 5 years, Kubernetes"}]`,
		})
		r := HandleSubmitTailorT2WithConfig(context.Background(), &t2Req, &cfgB, &config.Config{})
		if len(r.Content) == 0 {
			resB <- result{err: errors.New("T2: empty content")}
			return
		}
		tc, ok := r.Content[0].(mcp.TextContent)
		if !ok {
			resB <- result{err: errors.New("T2: content[0] not TextContent")}
			return
		}
		resB <- result{text: tc.Text}
	}()

	// Collect results.
	rA := <-resA
	rB := <-resB

	if rA.err != nil {
		t.Fatalf("T1 goroutine error: %v", rA.err)
	}
	if rB.err != nil {
		t.Fatalf("T2 goroutine error: %v", rB.err)
	}

	// Assert both envelopes are ok and carry tailored_text.
	for label, text := range map[string]string{"T1(sessionA)": rA.text, "T2(sessionB)": rB.text} {
		var env map[string]any
		if err := json.Unmarshal([]byte(text), &env); err != nil {
			t.Fatalf("%s response not JSON: %v — raw: %s", label, err, text)
		}
		if env["status"] != "ok" {
			t.Errorf("%s status = %v, want ok — full: %s", label, env["status"], text)
		}
		data, _ := env["data"].(map[string]any)
		if data == nil {
			t.Errorf("%s: expected data in response", label)
			continue
		}
		if _, ok := data["tailored_text"]; !ok {
			t.Errorf("%s: expected tailored_text in data — full: %s", label, text)
		}
	}

	// Verify session state persisted correctly.
	sA := sessions.Get(sessIDA)
	if sA == nil {
		t.Fatal("session A not found after T1")
	}
	if sA.State != stateT1Applied {
		t.Errorf("session A state = %v, want stateT1Applied", sA.State)
	}
	if sA.TailoredText == "" {
		t.Error("session A TailoredText is empty after T1")
	}

	sB := sessions.Get(sessIDB)
	if sB == nil {
		t.Fatal("session B not found after T2")
	}
	if sB.State != stateT2Applied {
		t.Errorf("session B state = %v, want stateT2Applied", sB.State)
	}
	if sB.TailoredText == "" {
		t.Error("session B TailoredText is empty after T2")
	}
}
