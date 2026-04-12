package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	loaderPkg "github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/presenter/headless"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/augment"
	"github.com/thedandano/go-apply/internal/service/coverletter"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

// newApplyCommand returns the `apply` cobra command wired with all services.
func newApplyCommand(defaults *config.AppDefaults) *cobra.Command {
	var (
		jdURL     string
		jdText    string
		channel   string
		resumeDir string
	)

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Score resumes against a job description and generate a cover letter",
		Long: `apply scores all resumes in the resume directory against a job description,
ranks them, optionally augments the best one, and generates a cover letter.

Output is JSON (headless mode). TUI mode will be added in a future release.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jdURL == "" && jdText == "" {
				return fmt.Errorf("either --url or --text must be provided")
			}
			if jdURL != "" && jdText != "" {
				return fmt.Errorf("--url and --text are mutually exclusive")
			}

			switch model.ChannelType(channel) {
			case model.ChannelCold, model.ChannelReferral, model.ChannelRecruiter:
				// valid
			default:
				return fmt.Errorf("invalid --channel %q: must be COLD, REFERRAL, or RECRUITER", channel)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// ── Presenter ──────────────────────────────────────────────────────
			pres := headless.New(os.Stdout)

			// ── Clients ────────────────────────────────────────────────────────
			llmClient := newLLMClient(cfg, defaults)
			embedderClient := newEmbedderClient(cfg, defaults)

			// ── Repositories ───────────────────────────────────────────────────
			resumeRepo := fs.NewResumeRepository(resumeDir)

			cacheDir := config.DataDir() + "/jd_cache"
			jdCacheRepo := fs.NewJDCacheRepository(cacheDir)

			// ── Augmenter (optional — degrades gracefully if DB unavailable) ──
			// Declared as port.Augmenter (interface) so that a nil value produces
			// a true nil interface rather than a (*augment.Service)(nil) typed nil,
			// which would satisfy `augmenter != nil` and panic on the nil receiver call.
			var augmenterSvc port.Augmenter
			profileRepo, dbErr := newSQLiteProfile(cfg, defaults)
			if dbErr == nil {
				augmenterSvc = augment.New(profileRepo, embedderClient, defaults)
			}

			// ── Services ───────────────────────────────────────────────────────
			scorerSvc := scorer.New(defaults)
			clGen := coverletter.New(llmClient, defaults)
			fetcherSvc := fetcher.NewFallbackFetcher(defaults)
			docLoader := loaderPkg.New()

			// ── Pipeline ───────────────────────────────────────────────────────
			p := pipeline.New(
				fetcherSvc,
				llmClient,
				scorerSvc,
				clGen,
				resumeRepo,
				jdCacheRepo,
				augmenterSvc, // may be nil
				docLoader,
				pres,
				defaults,
				cfg,
			)

			ch := model.ChannelType(channel)

			return p.Run(cmd.Context(), pipeline.RunInput{
				URL:     jdURL,
				Text:    jdText,
				Channel: ch,
			})
		},
	}

	cmd.Flags().StringVar(&jdURL, "url", "", "Job description URL to fetch")
	cmd.Flags().StringVar(&jdText, "text", "", "Job description text (use instead of --url)")
	cmd.Flags().StringVar(&channel, "channel", "COLD", "Application channel: COLD, REFERRAL, or RECRUITER")
	cmd.Flags().StringVar(&resumeDir, "resume-dir", ".", "Directory to scan for resumes (.pdf, .docx, .txt)")

	return cmd
}
