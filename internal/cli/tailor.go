package cli

import (
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/presenter/headless"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/augment"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
	tailorsvc "github.com/thedandano/go-apply/internal/service/tailor"
)

// NewTailorCommand returns the cobra command for "go-apply tailor".
// It tailors a specific resume against a job description using a two-tier cascade:
// tier-1 injects missing JD keywords into the Skills section;
// tier-2 rewrites relevant Experience bullets grounded in accomplishments (when provided).
func NewTailorCommand() *cobra.Command {
	var (
		resumeFlag          string
		urlFlag             string
		textFlag            string
		accomplishmentsFlag string
	)

	cmd := &cobra.Command{
		Use:   "tailor",
		Short: "Tailor a resume to better match a job description",
		Long: `Score and tailor a specific resume against a job description.
Tier-1 injects missing JD keywords into the Skills section.
Tier-2 rewrites relevant Experience bullets when --accomplishments is provided.
Outputs a JSON result to stdout.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if resumeFlag == "" {
				return fmt.Errorf("--resume is required")
			}
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

			defaults, err := config.LoadDefaults()
			if err != nil {
				return fmt.Errorf("load defaults: %w", err)
			}

			log := slog.Default()

			// Wire LLM clients.
			llmClient := llm.New(cfg.Orchestrator.BaseURL, cfg.Orchestrator.Model, cfg.Orchestrator.APIKey, defaults, log)
			embedderClient := llm.New(cfg.Embedder.BaseURL, cfg.Embedder.Model, cfg.Embedder.APIKey, defaults, log)

			// Wire repositories and loaders.
			dataDir := config.DataDir()
			appRepo := fs.NewApplicationRepository(dataDir)
			resumeRepo := fs.NewResumeRepository(dataDir)
			docLoader := loader.New()

			profileRepo, err := newSQLiteProfile(cfg)
			if err != nil {
				return err
			}

			// Wire services.
			augmentSvc := augment.New(profileRepo, profileRepo, embedderClient, llmClient, defaults, log)
			scorerSvc := scorer.New(defaults)
			fetcherSvc := fetcher.NewFallback(defaults, log)
			tailorSvc := tailorsvc.New(llmClient, defaults, log)

			// Wire presenter — headless JSON output.
			pres := headless.New()

			// Build and run the tailor pipeline.
			pl := pipeline.NewTailorPipeline(&pipeline.TailorConfig{
				Fetcher:   fetcherSvc,
				LLM:       llmClient,
				Scorer:    scorerSvc,
				Tailor:    tailorSvc,
				Resumes:   resumeRepo,
				Loader:    docLoader,
				AppRepo:   appRepo,
				Augment:   augmentSvc,
				Presenter: pres,
				Defaults:  defaults,
			})

			isText := textFlag != ""
			input := urlFlag
			if isText {
				input = textFlag
			}

			return pl.Run(cmd.Context(), pipeline.TailorRequest{
				URLOrText:           input,
				IsText:              isText,
				ResumeLabel:         resumeFlag,
				AccomplishmentsPath: accomplishmentsFlag,
				Config:              cfg,
			})
		},
	}

	cmd.Flags().StringVar(&resumeFlag, "resume", "", "Label of the resume to tailor (required)")
	cmd.Flags().StringVar(&urlFlag, "url", "", "URL of the job posting to fetch")
	cmd.Flags().StringVar(&textFlag, "text", "", "Raw job description text (alternative to --url)")
	cmd.Flags().StringVar(&accomplishmentsFlag, "accomplishments", "", "Path to accomplishments file for tier-2 bullet rewriting (optional)")

	_ = cmd.MarkFlagRequired("resume")

	return cmd
}
