package cli_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/model"
	fsrepo "github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/repository/sqlite"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/onboarding"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

// TestOnboardThenScore exercises the full onboard → score flow using real
// SQLite, real file I/O, real scorer, and stub HTTP servers for the LLM and
// embedder.  This test automates the manual flow from the first MCP session.
func TestOnboardThenScore(t *testing.T) {
	// ── 1. Setup ───────────────────────────────────────────────────────────────

	dataDir := t.TempDir()

	const embeddingDim = 3
	stubVector := []float32{0.1, 0.2, 0.3}

	// Stub embedder: returns a fixed 3-element embedding vector.
	embedderStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/embeddings" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"embedding": stubVector}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer embedderStub.Close()

	// Stub LLM: returns a structured JD JSON with go and kubernetes keywords.
	llmStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			jdJSON := `{"title":"Go Engineer","company":"Acme","required":["go","kubernetes"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{
					{"message": map[string]string{"content": jdJSON}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer llmStub.Close()

	defaults, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}

	log := slog.Default()

	// ── 2. Onboarding ──────────────────────────────────────────────────────────

	dbPath := filepath.Join(dataDir, "profile.db")
	profileRepo, err := sqlite.NewProfileRepository(dbPath, embeddingDim)
	if err != nil {
		t.Fatalf("NewProfileRepository: %v", err)
	}
	defer func() { _ = profileRepo.Close() }()

	embedderClient := llm.New(embedderStub.URL, "test-model", "", defaults, log)
	onboardSvc := onboarding.New(profileRepo, embedderClient, dataDir, log)

	resumeText := "golang kubernetes senior engineer five years experience"
	onboardResult, err := onboardSvc.Run(context.Background(), model.OnboardInput{
		Resumes: []model.ResumeEntry{{Label: "main", Text: resumeText}},
	})
	if err != nil {
		t.Fatalf("onboard Run: %v", err)
	}
	if len(onboardResult.Stored) == 0 {
		t.Fatalf("onboard stored nothing — warnings: %v", onboardResult.Warnings)
	}

	// Assert the resume file was written to disk.
	resumePath := filepath.Join(dataDir, "inputs", "main.txt")
	if _, statErr := os.Stat(resumePath); statErr != nil {
		t.Fatalf("resume file not written at %s: %v", resumePath, statErr)
	}

	// ── 3. Scoring ─────────────────────────────────────────────────────────────

	llmClient := llm.New(llmStub.URL, "test-model", "", defaults, log)
	scorerSvc := scorer.New(defaults)
	resumeRepo := fsrepo.NewResumeRepository(dataDir)
	docLoader := loader.New()
	appRepo := fsrepo.NewApplicationRepository(dataDir)

	deps := pipeline.ApplyConfig{
		Fetcher:   nil,
		LLM:       llmClient,
		Scorer:    scorerSvc,
		CLGen:     nil,
		Resumes:   resumeRepo,
		Loader:    docLoader,
		AppRepo:   appRepo,
		Augment:   nil,
		Defaults:  defaults,
		Tailor:    nil,
		Presenter: nil,
	}

	req := callToolRequest("get_score", map[string]any{
		"text":    "Senior Go Engineer at Acme. Requires: go, kubernetes. Nice to have: docker.",
		"channel": "COLD",
	})

	scoreResult := cli.HandleGetScore(context.Background(), &req, &deps)
	rawText := extractText(t, scoreResult)

	var pr model.PipelineResult
	if err := json.Unmarshal([]byte(rawText), &pr); err != nil {
		t.Fatalf("unmarshal PipelineResult: %v — raw: %s", err, rawText)
	}

	// ── 4. Assertions ──────────────────────────────────────────────────────────

	if pr.Status != "success" {
		t.Errorf("status = %q (error: %s), want success", pr.Status, pr.Error)
	}
	if pr.BestScore <= 0 {
		t.Errorf("best_score = %f, want > 0", pr.BestScore)
	}
	if pr.BestResume != "main" {
		t.Errorf("best_resume = %q, want main", pr.BestResume)
	}
}
