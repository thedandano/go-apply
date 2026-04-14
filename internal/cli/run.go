// Package cli provides Cobra commands for the go-apply CLI.
package cli

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/presenter/headless"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/augment"
	"github.com/thedandano/go-apply/internal/service/coverletter"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

// NewApplyCommand returns the cobra command for "go-apply run".
// It supports two mutually exclusive input modes:
//   - --url <url>  : fetch a job description from a URL
//   - --text <jd>  : provide raw JD text inline
//
// The --headless flag (default true for now) emits JSON to stdout.
func NewApplyCommand() *cobra.Command {
	var (
		urlFlag      string
		textFlag     string
		headlessFlag bool
		channelFlag  string
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

			defaults, err := config.LoadDefaults()
			if err != nil {
				return fmt.Errorf("load defaults: %w", err)
			}

			channel, err := resolveChannel(channelFlag)
			if err != nil {
				return err
			}

			log := slog.Default()

			// Wire LLM clients.
			llmClient := llm.New(cfg.Orchestrator.BaseURL, cfg.Orchestrator.Model, cfg.Orchestrator.APIKey, defaults, log)
			embedderClient := llm.New(cfg.Embedder.BaseURL, cfg.Embedder.Model, cfg.Embedder.APIKey, defaults, log)

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

			// Wire presenter — always headless for now.
			// TODO(Epic 6): swap in TUIPresenter when isatty detects a terminal and --headless is not set
			pres := headless.New()

			// Build and run the pipeline.
			pl := pipeline.NewApplyPipeline(
				fetcherSvc,
				llmClient,
				scorerSvc,
				clGen,
				resumeRepo,
				docLoader,
				appRepo,
				augmentSvc,
				pres,
				defaults,
			)

			isText := textFlag != ""
			input := urlFlag
			if isText {
				input = textFlag
			}

			return pl.Run(cmd.Context(), pipeline.ApplyRequest{
				URLOrText: input,
				IsText:    isText,
				Channel:   channel,
				Config:    cfg,
			})
		},
	}

	cmd.Flags().StringVar(&urlFlag, "url", "", "URL of the job posting to fetch")
	cmd.Flags().StringVar(&textFlag, "text", "", "Raw job description text (alternative to --url)")
	cmd.Flags().BoolVar(&headlessFlag, "headless", true, "Output JSON to stdout (default; TUI available in future)")
	cmd.Flags().StringVar(&channelFlag, "channel", "COLD", "Application channel: COLD, REFERRAL, or RECRUITER")

	return cmd
}

// resolveChannel parses a channel string into a model.ChannelType.
func resolveChannel(s string) (model.ChannelType, error) {
	switch strings.ToUpper(s) {
	case "COLD":
		return model.ChannelCold, nil
	case "REFERRAL":
		return model.ChannelReferral, nil
	case "RECRUITER":
		return model.ChannelRecruiter, nil
	default:
		return "", fmt.Errorf("unknown channel %q — valid values: COLD, REFERRAL, RECRUITER", s)
	}
}
