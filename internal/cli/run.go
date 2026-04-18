// Package cli provides Cobra commands for the go-apply CLI.
package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/presenter/headless"
	"github.com/thedandano/go-apply/internal/redact"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/augment"
	"github.com/thedandano/go-apply/internal/service/coverletter"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/orchestrator"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
	"github.com/thedandano/go-apply/internal/service/tailor"
)

// NewApplyCommand returns the cobra command for "go-apply run".
// It supports two mutually exclusive input modes:
//   - --url <url>  : fetch a job description from a URL
//   - --text <jd>  : provide raw JD text inline
//
// The --headless flag (default true for now) emits JSON to stdout.
func NewApplyCommand() *cobra.Command {
	var (
		urlFlag             string
		textFlag            string
		headlessFlag        bool
		channelFlag         string
		accomplishmentsFlag string
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the full apply pipeline for a job description",
		Long: `Fetch or accept a job description, score your resumes, and generate a cover letter.
Outputs a JSON result to stdout when --headless is set.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if urlFlag == "" && textFlag == "" {
				return fmt.Errorf("one of --url or --text is required")
			}
			if urlFlag != "" && textFlag != "" {
				return fmt.Errorf("--url and --text are mutually exclusive")
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if cfg.Orchestrator.BaseURL == "" || cfg.Orchestrator.Model == "" {
				return fmt.Errorf("no orchestrator configured — run:\n  go-apply config set llm.base_url <url>\n  go-apply config set llm.model <model>")
			}

			r := redact.New(&redact.Profile{
				Name:        cfg.UserName,
				Location:    cfg.Location,
				LinkedInURL: cfg.LinkedInURL,
			})
			logger.SetRedactor(r)

			defaults, err := config.LoadDefaults()
			if err != nil {
				return fmt.Errorf("load defaults: %w", err)
			}
			cfg.ApplyOverrides(defaults)

			channel, err := model.ParseChannel(channelFlag)
			if err != nil {
				return err
			}

			log := slog.Default()

			// Wire LLM clients.
			llmClient := llm.New(cfg.Orchestrator.BaseURL, cfg.Orchestrator.Model, cfg.Orchestrator.APIKey, defaults, log)
			embedderClient := llm.New(cfg.Embedder.BaseURL, cfg.Embedder.Model, cfg.Embedder.APIKey, defaults, log)

			// Wire orchestrator for CLI/TUI mode.
			orch := orchestrator.NewLLMOrchestrator(llmClient)

			// Wire repositories.
			dataDir := config.DataDir()
			appRepo := fs.NewApplicationRepository(dataDir)
			resumeRepo := fs.NewResumeRepository(dataDir)
			docLoader := loader.New()

			// Wire profile DB (implements both port.ProfileRepository and port.KeywordCacheRepository).
			profileRepo, err := newSQLiteProfile(cfg)
			if err != nil {
				return err
			}

			// Wire services.
			augmentSvc := augment.New(profileRepo, profileRepo, embedderClient, llmClient, defaults, log)
			scorerSvc := scorer.New(defaults)
			clGen := coverletter.New(llmClient, defaults, log)
			fetcherSvc := fetcher.NewFallback(defaults, log)
			tailorSvc := tailor.New(llmClient, defaults, log)

			// Wire presenter — always headless for now.
			// TODO(Epic 6): swap in TUIPresenter when isatty detects a terminal and --headless is not set
			pres := headless.New()

			// Build and run the pipeline.
			pl := pipeline.NewApplyPipeline(&pipeline.ApplyConfig{
				Fetcher:      fetcherSvc,
				LLM:          llmClient,
				Scorer:       scorerSvc,
				CLGen:        clGen,
				Resumes:      resumeRepo,
				Loader:       docLoader,
				AppRepo:      appRepo,
				Augment:      augmentSvc,
				Presenter:    pres,
				Defaults:     defaults,
				Tailor:       tailorSvc,
				Orchestrator: orch,
			})

			isText := textFlag != ""
			input := urlFlag
			if isText {
				input = textFlag
			}

			var accomplishmentsText string
			if accomplishmentsFlag != "" {
				data, err := os.ReadFile(accomplishmentsFlag) // #nosec G304 -- path is explicitly provided by the user via --accomplishments flag
				if err != nil {
					return fmt.Errorf("read accomplishments file: %w", err)
				}
				accomplishmentsText = string(data)
			}

			return pl.Run(cmd.Context(), pipeline.ApplyRequest{
				URLOrText:           input,
				IsText:              isText,
				Channel:             channel,
				Config:              cfg,
				AccomplishmentsText: accomplishmentsText,
			})
		},
	}

	cmd.Flags().StringVar(&urlFlag, "url", "", "URL of the job posting to fetch")
	cmd.Flags().StringVar(&textFlag, "text", "", "Raw job description text (alternative to --url)")
	cmd.Flags().BoolVar(&headlessFlag, "headless", true, "Output JSON to stdout (default; TUI available in future)")
	cmd.Flags().StringVar(&channelFlag, "channel", "COLD", "Application channel: COLD, REFERRAL, or RECRUITER")
	cmd.Flags().StringVar(&accomplishmentsFlag, "accomplishments", "", "Path to accomplishments doc for tier-2 bullet rewriting (optional)")

	return cmd
}
