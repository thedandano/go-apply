package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/onboarding"
)

// NewOnboardCommand returns the cobra command for "go-apply onboard".
// It accepts one or more resume files plus optional skills and accomplishments paths.
func NewOnboardCommand() *cobra.Command {
	var (
		resumePaths         []string
		skillsFlag          string
		accomplishmentsFlag string
	)

	cmd := &cobra.Command{
		Use:   "onboard",
		Short: "Store resumes, skills, and accomplishments in the profile database",
		Long: `Read resume, skills, and accomplishments files, embed them, and store
them in the local profile database for use during the apply pipeline.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(resumePaths) == 0 && skillsFlag == "" && accomplishmentsFlag == "" {
				return fmt.Errorf("at least one of --resume, --skills, or --accomplishments is required")
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			docLoader := loader.New()

			// Build resume entries, checking for duplicate labels.
			var resumes []model.ResumeEntry
			seenLabels := make(map[string]string) // label → original path
			for _, path := range resumePaths {
				label := labelFromPath(path)
				if existing, dup := seenLabels[label]; dup {
					return fmt.Errorf("duplicate resume label %q: paths %q and %q produce the same label", label, existing, path)
				}
				seenLabels[label] = path

				text, err := docLoader.Load(path)
				if err != nil {
					return fmt.Errorf("load resume %s: %w", path, err)
				}
				resumes = append(resumes, model.ResumeEntry{Label: label, Text: text})
			}

			var skillsText string
			if skillsFlag != "" {
				data, err := os.ReadFile(skillsFlag) // #nosec G304 -- path is explicitly provided by the user via --skills flag
				if err != nil {
					return fmt.Errorf("read skills file: %w", err)
				}
				skillsText = string(data)
			}

			var accomplishmentsText string
			if accomplishmentsFlag != "" {
				data, err := os.ReadFile(accomplishmentsFlag) // #nosec G304 -- path is explicitly provided by the user via --accomplishments flag
				if err != nil {
					return fmt.Errorf("read accomplishments file: %w", err)
				}
				accomplishmentsText = string(data)
			}

			defaults, err := config.LoadDefaults()
			if err != nil {
				return fmt.Errorf("load defaults: %w", err)
			}

			log := slog.Default()
			embedderClient := llm.New(cfg.Embedder.BaseURL, cfg.Embedder.Model, cfg.Embedder.APIKey, defaults, log)

			profileRepo, err := newSQLiteProfile(cfg)
			if err != nil {
				return err
			}

			svc := onboarding.New(profileRepo, embedderClient, config.DataDir(), log)

			result, err := svc.Run(cmd.Context(), model.OnboardInput{
				Resumes:             resumes,
				SkillsText:          skillsText,
				AccomplishmentsText: accomplishmentsText,
			})
			if err != nil {
				return fmt.Errorf("onboard: %w", err)
			}

			data, err := json.Marshal(result)
			if err != nil {
				return fmt.Errorf("marshal result: %w", err)
			}
			fmt.Println(string(data))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&resumePaths, "resume", nil, "Path to a resume file (repeatable)")
	cmd.Flags().StringVar(&skillsFlag, "skills", "", "Path to skills reference file")
	cmd.Flags().StringVar(&accomplishmentsFlag, "accomplishments", "", "Path to accomplishments file")

	return cmd
}

// labelFromPath returns the filename stem (no extension, no directory) for a resume path.
func labelFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	label := strings.TrimSuffix(base, ext)
	return label
}
