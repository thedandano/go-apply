package pipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/presenter/headless"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

// stubTailorService satisfies port.Tailor with a fixed result.
type stubTailorService struct{}

var _ port.Tailor = (*stubTailorService)(nil)

func (s *stubTailorService) TailorResume(_ context.Context, input *model.TailorInput) (model.TailorResult, error) {
	return model.TailorResult{
		ResumeLabel:   input.Resume.Label,
		TierApplied:   model.TierKeyword,
		AddedKeywords: []string{"Golang"},
		TailoredText:  input.ResumeText + "\nGolang",
	}, nil
}

func TestTailorPipeline_ShowTailorResultCalled(t *testing.T) {
	llmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"choices": []map[string]any{
				{"message": map[string]any{
					"content": `{"title":"SWE","company":"Acme","required":["Golang","Kubernetes"],"preferred":["Docker"],"location":"Remote","seniority":"senior","required_years":3}`,
				}},
			},
		})
	}))
	defer llmSrv.Close()

	var stdout, stderr bytes.Buffer
	pres := headless.NewWith(&stdout, &stderr)

	defaults, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}

	llmClient := llm.New(llmSrv.URL, "test", "test", defaults, nil)

	cfg := &config.Config{
		Orchestrator:      config.LLMProviderConfig{BaseURL: llmSrv.URL, Model: "test", APIKey: "test"},
		YearsOfExperience: 5,
		DefaultSeniority:  "senior",
	}

	pl := pipeline.NewTailorPipeline(&pipeline.TailorConfig{
		Fetcher:   &stubJDFetcher{},
		LLM:       llmClient,
		Scorer:    scorer.New(defaults),
		Tailor:    &stubTailorService{},
		Resumes:   &stubResumeRepo{},
		Loader:    &stubDocumentLoader{},
		AppRepo:   &stubAppRepo{},
		Augment:   &stubAugmentService{},
		Presenter: pres,
		Defaults:  defaults,
	})

	err = pl.Run(context.Background(), pipeline.TailorRequest{
		URLOrText:   `Senior Go Engineer role. Required: Golang, Kubernetes. Preferred: Docker.`,
		IsText:      true,
		ResumeLabel: "test",
		Config:      cfg,
	})

	if err != nil {
		t.Fatalf("TailorPipeline.Run error: %v", err)
	}

	// Verify ShowTailorResult wrote to stdout.
	if stdout.Len() == 0 {
		t.Fatal("ShowTailorResult was not called — stdout is empty")
	}

	var result model.TailorResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not valid TailorResult JSON: %v\nOutput: %s", err, stdout.String())
	}
	if result.ResumeLabel == "" {
		t.Error("TailorResult.ResumeLabel must not be empty")
	}
}
