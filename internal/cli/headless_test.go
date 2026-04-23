package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/sessionstore"

	"github.com/mark3labs/mcp-go/mcp"
)

// ── test helpers ──────────────────────────────────────────────────────────────

// executeHeadless runs the root command with the given args under a temp XDG environment
// and returns (stdout, stderr, error).
func executeHeadless(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	root := cli.NewRootCommand("test")
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// setupTempXDG redirects XDG_DATA_HOME and XDG_CONFIG_HOME to temp dirs.
// Returns the temp data dir path.
func setupTempXDG(t *testing.T) string {
	t.Helper()
	tmpData := t.TempDir()
	tmpConfig := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmpData)
	t.Setenv("XDG_CONFIG_HOME", tmpConfig)
	return tmpData
}

// headlessStubs for CLI tests — these are test doubles matching the port interfaces.
type headlessStubJDFetcher struct{}

var _ port.JDFetcher = (*headlessStubJDFetcher)(nil)

func (s *headlessStubJDFetcher) Fetch(_ context.Context, _ string) (string, error) {
	return "fake job description text from fetcher", nil
}

type headlessStubScorer struct{}

var _ port.Scorer = (*headlessStubScorer)(nil)

func (s *headlessStubScorer) Score(_ *model.ScorerInput) (model.ScoreResult, error) {
	return model.ScoreResult{
		Breakdown: model.ScoreBreakdown{
			KeywordMatch:   0.8,
			ExperienceFit:  0.8,
			ImpactEvidence: 0.8,
			ATSFormat:      0.8,
			Readability:    0.8,
		},
	}, nil
}

type headlessStubResumeRepo struct{}

var _ port.ResumeRepository = (*headlessStubResumeRepo)(nil)

func (s *headlessStubResumeRepo) ListResumes() ([]model.ResumeFile, error) {
	return []model.ResumeFile{{Label: "main", Path: "/fake/main.txt"}}, nil
}

type headlessStubDocumentLoader struct{}

var _ port.DocumentLoader = (*headlessStubDocumentLoader)(nil)

func (s *headlessStubDocumentLoader) Load(_ string) (string, error) {
	return "golang senior engineer 5 years experience", nil
}
func (s *headlessStubDocumentLoader) Supports(_ string) bool { return true }

type headlessStubAppRepo struct{}

var _ port.ApplicationRepository = (*headlessStubAppRepo)(nil)

func (s *headlessStubAppRepo) Get(_ string) (*model.ApplicationRecord, bool, error) {
	return nil, false, nil
}
func (s *headlessStubAppRepo) Put(_ *model.ApplicationRecord) error    { return nil }
func (s *headlessStubAppRepo) Update(_ *model.ApplicationRecord) error { return nil }
func (s *headlessStubAppRepo) List() ([]*model.ApplicationRecord, error) {
	return nil, nil
}

// capturingAppRepo records the last Put call for assertion in integration tests.
type capturingAppRepo struct {
	puts []*model.ApplicationRecord
}

var _ port.ApplicationRepository = (*capturingAppRepo)(nil)

func (c *capturingAppRepo) Get(_ string) (*model.ApplicationRecord, bool, error) {
	return nil, false, nil
}
func (c *capturingAppRepo) Put(rec *model.ApplicationRecord) error {
	c.puts = append(c.puts, rec)
	return nil
}
func (c *capturingAppRepo) Update(_ *model.ApplicationRecord) error { return nil }
func (c *capturingAppRepo) List() ([]*model.ApplicationRecord, error) {
	return nil, nil
}

// stubApplyConfig returns a pipeline.ApplyConfig wired with test doubles.
func headlessStubConfig() pipeline.ApplyConfig {
	return pipeline.ApplyConfig{
		Fetcher:  &headlessStubJDFetcher{},
		Scorer:   &headlessStubScorer{},
		Resumes:  &headlessStubResumeRepo{},
		Loader:   &headlessStubDocumentLoader{},
		AppRepo:  &headlessStubAppRepo{},
		Defaults: &config.AppDefaults{},
	}
}

// handlerScore invokes HandleSubmitKeywordsWithConfig with the given store.
func handlerScore(t *testing.T, store sessionstore.Store, sessionID, jdJSON string) {
	t.Helper()
	cfg := headlessStubConfig()
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "submit_keywords",
			Arguments: map[string]any{
				"session_id": sessionID,
				"jd_json":    jdJSON,
			},
		},
	}
	result := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &req, &cfg, nil, store)
	if len(result.Content) == 0 {
		t.Fatal("HandleSubmitKeywordsWithConfig returned no content")
	}
	tc := result.Content[0].(mcp.TextContent)
	var env map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &env); err != nil {
		t.Fatalf("submit_keywords response not JSON: %v", err)
	}
	if env["status"] != "ok" {
		t.Fatalf("submit_keywords failed: %s", tc.Text)
	}
}

// ── load-jd command tests ─────────────────────────────────────────────────────

func TestLoadJDCommand_NoArgs_ReturnsError(t *testing.T) {
	setupTempXDG(t)
	_, stderr, err := executeHeadless(t, "load-jd")
	if err == nil {
		t.Error("expected error with no args")
	}
	// Should print cobra usage error (required flag missing) or our JSON error.
	_ = stderr
}

func TestLoadJDCommand_BothArgs_ReturnsError(t *testing.T) {
	setupTempXDG(t)
	_, stderr, err := executeHeadless(t, "load-jd", "--url", "https://example.com/job", "--text", "raw text")
	if err == nil {
		t.Error("expected error with both args")
	}
	// Stderr should have JSON error.
	if stderr != "" {
		var env map[string]any
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); jsonErr == nil {
			if env["status"] != "error" {
				t.Errorf("expected error status, got %v", env["status"])
			}
			if env["code"] != "invalid_input" {
				t.Errorf("expected code invalid_input, got %v", env["code"])
			}
		}
	}
}

func TestLoadJDCommand_SessionFlag_ReturnsUnsupportedFlag(t *testing.T) {
	setupTempXDG(t)
	// writeError writes JSON to os.Stderr (not cobra's err buffer) and returns a non-nil
	// error. Cobra captures the returned error and formats it into its err buffer as
	// "Error: <err.Error()>". We assert that the cobra error message contains the
	// sentinel code "unsupported_flag" and that the command exits non-zero.
	_, stderr, err := executeHeadless(t, "load-jd", "--session", "abc", "--text", "some job description text")
	if err == nil {
		t.Error("expected error when --session is passed to load-jd")
	}
	// The cobra err buffer will contain "Error: unsupported_flag: ..." because writeError
	// returns fmt.Errorf("%s: %s", code, message) to PreRunE which cobra formats.
	if !strings.Contains(stderr, "unsupported_flag") {
		t.Errorf("expected stderr to contain %q; got: %q", "unsupported_flag", stderr)
	}
}

// ── score command tests ───────────────────────────────────────────────────────

func TestScoreCommand_MissingSession_ReturnsError(t *testing.T) {
	setupTempXDG(t)
	// score --session needs a real session file. A missing session returns session_not_found.
	_, stderr, err := executeHeadless(t, "score",
		"--session", "deadbeefdeadbeefdeadbeefdeadbeef",
		"--jd-json", `{"title":"Go Engineer","required":["go"]}`,
	)
	if err == nil {
		t.Error("expected error for missing session")
	}
	if stderr != "" {
		var env map[string]any
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env); jsonErr == nil {
			if env["status"] != "error" {
				t.Errorf("expected error status, got %v in %s", env["status"], stderr)
			}
		}
	}
}

// ── finalize command tests ────────────────────────────────────────────────────

func TestFinalizeCommand_MissingSession_ReturnsError(t *testing.T) {
	setupTempXDG(t)
	_, _, err := executeHeadless(t, "finalize",
		"--session", "deadbeefdeadbeefdeadbeefdeadbeef",
	)
	if err == nil {
		t.Error("expected error for missing session")
	}
}

// ── submit-tailored-resume command tests ─────────────────────────────────────

func TestSubmitTailoredResumeCommand_MissingFile_ReturnsError(t *testing.T) {
	setupTempXDG(t)
	_, _, err := executeHeadless(t, "submit-tailored-resume",
		"--session", "deadbeefdeadbeefdeadbeefdeadbeef",
		"--tailored-text-file", "/nonexistent/path/resume.txt",
	)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

// ── DiskStore integration: CLI subcommands chain via shared disk store ────────

// TestDiskStore_HeadlessHandlerChain tests that the handlers correctly read and write
// session state through the DiskStore — simulating what the CLI subcommands do.
// Uses a URL-based load so that finalize exercises the AppRepo.Put path.
func TestDiskStore_HeadlessHandlerChain(t *testing.T) {
	tmpData := setupTempXDG(t)
	sessDir := filepath.Join(tmpData, "go-apply", "sessions")

	store, err := sessionstore.NewDiskStore(sessDir)
	if err != nil {
		t.Fatalf("NewDiskStore: %v", err)
	}

	ctx := context.Background()

	// Wire a capturing AppRepo so we can assert AppRepo.Put is called during finalize.
	appRepo := &capturingAppRepo{}
	cfg := pipeline.ApplyConfig{
		Fetcher:  &headlessStubJDFetcher{},
		Scorer:   &headlessStubScorer{},
		Resumes:  &headlessStubResumeRepo{},
		Loader:   &headlessStubDocumentLoader{},
		AppRepo:  appRepo,
		Defaults: &config.AppDefaults{},
	}

	// Step 1: load-jd via URL (so sess.URL != "" and AppRepo.Put fires in finalize).
	urlReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "load_jd",
			Arguments: map[string]any{"jd_url": "https://example.com/job/go-engineer"},
		},
	}
	urlResult := mcpserver.HandleLoadJDWithConfig(ctx, &urlReq, &cfg, store)
	if len(urlResult.Content) == 0 {
		t.Fatal("load_jd (url) returned no content")
	}
	tc := urlResult.Content[0].(mcp.TextContent)
	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &loadEnv); err != nil {
		t.Fatalf("load_jd response not JSON: %v", err)
	}
	if loadEnv["status"] != "ok" {
		t.Fatalf("load_jd failed: %s", tc.Text)
	}
	sessionID := loadEnv["session_id"].(string)

	// Verify session file exists on disk.
	sessionFile := filepath.Join(sessDir, sessionID+".json")
	if _, err := os.Stat(sessionFile); err != nil {
		t.Fatalf("session file not found after load-jd: %v", err)
	}

	// Verify URL was persisted.
	loadedSess, ok, err := store.Get(ctx, sessionID)
	if err != nil || !ok {
		t.Fatalf("Get after load-jd: err=%v ok=%v", err, ok)
	}
	if loadedSess.URL != "https://example.com/job/go-engineer" {
		t.Errorf("sess.URL = %q, want URL set", loadedSess.URL)
	}

	// Step 2: score.
	jdJSON := `{"title":"Go Engineer","company":"Acme","required":["go","kubernetes"],"preferred":[]}`
	handlerScore(t, store, sessionID, jdJSON)

	// Verify state is scored on disk.
	sess, ok, err := store.Get(ctx, sessionID)
	if err != nil || !ok {
		t.Fatalf("Get after score: err=%v ok=%v", err, ok)
	}
	if sess.State != sessionstore.StateScored {
		t.Errorf("state after score = %q, want %q", sess.State, sessionstore.StateScored)
	}

	// Step 3: submit-tailored-resume with a non-empty changelog so that finalize
	// populates TailorResult on the ApplicationRecord.
	changelogJSON := `[{"action":"added","target":"skill","keyword":"kubernetes","reason":"required by JD"}]`
	tailoredReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "submit_tailored_resume",
			Arguments: map[string]any{
				"session_id":    sessionID,
				"tailored_text": "Tailored resume with go and kubernetes experience.",
				"changelog":     changelogJSON,
			},
		},
	}
	tailoredResult := mcpserver.HandleSubmitTailoredResumeWithConfig(ctx, &tailoredReq, &cfg, nil, store)
	if len(tailoredResult.Content) == 0 {
		t.Fatal("submit_tailored_resume returned no content")
	}
	tc = tailoredResult.Content[0].(mcp.TextContent)
	var tailoredEnv map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &tailoredEnv); err != nil {
		t.Fatalf("submit_tailored_resume not JSON: %v", err)
	}
	if tailoredEnv["status"] != "ok" {
		t.Fatalf("submit_tailored_resume failed: %s", tc.Text)
	}

	// Step 4: finalize.
	finalReq := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "finalize",
			Arguments: map[string]any{
				"session_id":   sessionID,
				"cover_letter": "Dear Hiring Manager...",
			},
		},
	}
	finalResult := mcpserver.HandleFinalizeWithConfig(ctx, &finalReq, &cfg, store)
	if len(finalResult.Content) == 0 {
		t.Fatal("finalize returned no content")
	}
	tc = finalResult.Content[0].(mcp.TextContent)
	var finalEnv map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &finalEnv); err != nil {
		t.Fatalf("finalize not JSON: %v", err)
	}
	if finalEnv["status"] != "ok" {
		t.Fatalf("finalize failed: %s", tc.Text)
	}

	// Verify AppRepo.Put was called (URL-based flow triggers persistence).
	if len(appRepo.puts) == 0 {
		t.Fatal("AppRepo.Put was not called during finalize for URL-based session")
	}
	rec := appRepo.puts[0]
	if rec.URL != "https://example.com/job/go-engineer" {
		t.Errorf("persisted record URL = %q, want %q", rec.URL, "https://example.com/job/go-engineer")
	}
	if rec.CoverLetter != "Dear Hiring Manager..." {
		t.Errorf("persisted record CoverLetter = %q, want set", rec.CoverLetter)
	}
	// ResumeLabel must be set (the stub scorer's best resume label).
	if rec.ResumeLabel == "" {
		t.Error("persisted record ResumeLabel must not be empty")
	}
	// Score must be non-nil and have a positive total.
	if rec.Score == nil {
		t.Error("persisted record Score must not be nil")
	} else if total := rec.Score.Breakdown.Total(); total <= 0 {
		t.Errorf("persisted record Score.Breakdown.Total() = %v, want > 0", total)
	}
	// TailorResult must be non-nil (changelog was submitted in step 3).
	if rec.TailorResult == nil {
		t.Error("persisted record TailorResult must not be nil (changelog was submitted)")
	} else {
		if len(rec.TailorResult.Changelog) == 0 {
			t.Error("TailorResult.Changelog must not be empty")
		} else {
			entry := rec.TailorResult.Changelog[0]
			if entry.Action != "added" {
				t.Errorf("Changelog[0].Action = %q, want %q", entry.Action, "added")
			}
			if entry.Target != "skill" {
				t.Errorf("Changelog[0].Target = %q, want %q", entry.Target, "skill")
			}
			if entry.Keyword != "kubernetes" {
				t.Errorf("Changelog[0].Keyword = %q, want %q", entry.Keyword, "kubernetes")
			}
		}
	}

	// After finalize, manually delete the session (simulating CLI finalize behavior).
	if err := store.Delete(ctx, sessionID); err != nil {
		t.Fatalf("Delete after finalize: %v", err)
	}

	// Session file must be gone.
	if _, statErr := os.Stat(sessionFile); statErr == nil {
		t.Error("session file still exists after finalize+delete")
	}
}

// TestDiskStore_SessionRoundTrip verifies that all session fields survive a disk round-trip.
func TestDiskStore_SessionRoundTrip(t *testing.T) {
	tmpData := setupTempXDG(t)
	sessDir := filepath.Join(tmpData, "go-apply", "sessions")

	store, err := sessionstore.NewDiskStore(sessDir)
	if err != nil {
		t.Fatalf("NewDiskStore: %v", err)
	}
	ctx := context.Background()

	sess, err := store.Create(ctx, "jd text for round-trip")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sess.URL = "https://example.com/job"
	sess.IsText = false
	sess.State = sessionstore.StateTailored
	sess.TailoredText = "tailored resume text"
	sess.Changelog = []model.ChangelogEntry{
		{Action: "added", Target: "skill", Keyword: "kubernetes", Reason: "required"},
	}
	if err := store.Update(ctx, sess); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, ok, err := store.Get(ctx, sess.ID)
	if err != nil || !ok {
		t.Fatalf("Get after Update: err=%v ok=%v", err, ok)
	}

	// Verify all fields.
	if got.ID != sess.ID {
		t.Errorf("ID: got %q want %q", got.ID, sess.ID)
	}
	if got.URL != sess.URL {
		t.Errorf("URL: got %q want %q", got.URL, sess.URL)
	}
	if got.IsText != sess.IsText {
		t.Errorf("IsText: got %v want %v", got.IsText, sess.IsText)
	}
	if got.JDText != sess.JDText {
		t.Errorf("JDText: got %q want %q", got.JDText, sess.JDText)
	}
	if got.State != sess.State {
		t.Errorf("State: got %q want %q", got.State, sess.State)
	}
	if got.TailoredText != sess.TailoredText {
		t.Errorf("TailoredText: got %q want %q", got.TailoredText, sess.TailoredText)
	}
	if len(got.Changelog) != 1 {
		t.Fatalf("Changelog length: got %d want 1", len(got.Changelog))
	}
	if got.Changelog[0].Keyword != "kubernetes" {
		t.Errorf("Changelog[0].Keyword: got %q want %q", got.Changelog[0].Keyword, "kubernetes")
	}
	if !got.CreatedAt.Equal(sess.CreatedAt) {
		t.Errorf("CreatedAt: got %v want %v", got.CreatedAt, sess.CreatedAt)
	}
}
