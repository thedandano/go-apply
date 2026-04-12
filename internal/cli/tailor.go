package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	loaderPkg "github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/presenter/headless"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/augment"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
	tailorSvc "github.com/thedandano/go-apply/internal/service/tailor"
)

// newTailorCommand returns the `tailor` cobra command wired with all services.
func newTailorCommand(defaults *config.AppDefaults) *cobra.Command {
	var (
		resumeLabel         string
		jdURL               string
		jdText              string
		resumeDir           string
		accomplishmentsPath string
	)

	cmd := &cobra.Command{
		Use:   "tailor",
		Short: "Tailor a resume against a job description using a two-tier keyword/bullet cascade",
		Long: `tailor applies two tiers of modifications to a resume:
  Tier 1: injects missing required keywords into the skills section.
  Tier 2: rewrites experience bullets via LLM, grounded in your accomplishments document.

Output is JSON (headless mode).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jdURL == "" && jdText == "" {
				return fmt.Errorf("either --url or --text must be provided")
			}
			if jdURL != "" && jdText != "" {
				return fmt.Errorf("--url and --text are mutually exclusive")
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

			cacheDir := filepath.Join(config.DataDir(), "jd_cache")
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
			fetcherSvc := fetcher.NewFallbackFetcher(defaults)
			docLoader := loaderPkg.New()
			tailor := tailorSvc.New(llmClient, defaults)

			// ── Pipeline ───────────────────────────────────────────────────────
			pl := pipeline.NewTailorPipeline(pipeline.TailorConfig{
				Fetcher:   fetcherSvc,
				LLM:       llmClient,
				Scorer:    scorerSvc,
				Tailor:    tailor,
				Resumes:   resumeRepo,
				JDCache:   jdCacheRepo,
				DocLoader: docLoader,
				Augmenter: augmenterSvc,
				Presenter: pres,
				Defaults:  defaults,
			})

			return pl.Run(cmd.Context(), pipeline.TailorRequest{
				ResumeLabel:         resumeLabel,
				URL:                 jdURL,
				Text:                jdText,
				AccomplishmentsPath: accomplishmentsPath,
				Cfg:                 cfg,
			})
		},
	}

	cmd.Flags().StringVar(&resumeLabel, "resume", "", "Resume label to tailor (required)")
	cmd.Flags().StringVar(&jdURL, "url", "", "Job description URL to fetch")
	cmd.Flags().StringVar(&jdText, "text", "", "Job description text (use instead of --url)")
	cmd.Flags().StringVar(&resumeDir, "resume-dir", ".", "Directory to scan for resumes (.pdf, .docx, .txt)")
	cmd.Flags().StringVar(&accomplishmentsPath, "accomplishments", "", "Path to accomplishments document for Tier 2 bullet rewrites")

	if err := cmd.MarkFlagRequired("resume"); err != nil {
		panic(fmt.Sprintf("failed to mark --resume as required: %v", err))
	}

	return cmd
}
