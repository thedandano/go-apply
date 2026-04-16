//go:build integration

package mcpserver_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	fsrepo "github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/repository/sqlite"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/onboarding"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

// TestOnboardThenScore exercises the full onboard → load_jd → submit_keywords flow
// using real SQLite, real file I/O, real scorer, and a stub embedder.
// Keyword extraction is performed by the test (mimicking the MCP host role), so no
// LLM stub is needed for scoring.
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

	resumePath := filepath.Join(dataDir, "inputs", "main.txt")
	if _, statErr := os.Stat(resumePath); statErr != nil {
		t.Fatalf("resume file not written at %s: %v", resumePath, statErr)
	}

	// ── 3. Build pipeline deps (no LLM — MCP host extracts keywords) ───────────

	scorerSvc := scorer.New(defaults)
	resumeRepo := fsrepo.NewResumeRepository(dataDir)
	docLoader := loader.New()
	appRepo := fsrepo.NewApplicationRepository(dataDir)

	deps := pipeline.ApplyConfig{
		Fetcher:   nil,
		LLM:       nil, // MCP host (this test) provides keywords via submit_keywords
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

	// ── 4. load_jd ─────────────────────────────────────────────────────────────

	loadReq := callToolRequest("load_jd", map[string]any{
		"jd_raw_text": "Senior Go Engineer at Acme. Requires: go, kubernetes. Nice to have: docker.",
	})
	loadResult := mcpserver.HandleLoadJDWithConfig(context.Background(), &loadReq, &deps)
	loadText := extractText(t, loadResult)

	var loadEnv map[string]any
	if err := json.Unmarshal([]byte(loadText), &loadEnv); err != nil {
		t.Fatalf("load_jd response not JSON: %v — raw: %s", err, loadText)
	}
	if loadEnv["status"] != "ok" {
		t.Fatalf("load_jd status = %v, want ok — full: %s", loadEnv["status"], loadText)
	}
	sessionID, _ := loadEnv["session_id"].(string)
	if sessionID == "" {
		t.Fatal("load_jd returned no session_id")
	}

	// ── 5. submit_keywords (test plays the MCP host role) ─────────────────────

	const jdJSON = `{"title":"Go Engineer","company":"Acme","required":["go","kubernetes"],"preferred":["docker"],"location":"Remote","seniority":"senior","required_years":3}`
	kwReq := callToolRequest("submit_keywords", map[string]any{
		"session_id": sessionID,
		"jd_json":    jdJSON,
	})
	kwResult := mcpserver.HandleSubmitKeywordsWithConfig(context.Background(), &kwReq, &deps, &config.Config{})
	kwText := extractText(t, kwResult)

	var kwEnv map[string]any
	if err := json.Unmarshal([]byte(kwText), &kwEnv); err != nil {
		t.Fatalf("submit_keywords response not JSON: %v — raw: %s", err, kwText)
	}

	// ── 6. Assertions ──────────────────────────────────────────────────────────

	if kwEnv["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", kwEnv["status"], kwText)
	}
	data, _ := kwEnv["data"].(map[string]any)
	if data == nil {
		t.Fatalf("expected data in submit_keywords response")
	}
	bestScore, _ := data["best_score"].(float64)
	if bestScore <= 0 {
		t.Errorf("best_score = %v, want > 0", data["best_score"])
	}
	if data["best_resume"] != "main" {
		t.Errorf("best_resume = %v, want main", data["best_resume"])
	}
}
