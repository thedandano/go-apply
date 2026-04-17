package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/onboarding"
)

// NewOnboardCommand returns the cobra command for "go-apply onboard".
// It accepts one or more resume files plus optional skills and accomplishments paths.
// When --reset is set, it clears the profile database and inputs/ directory.
func NewOnboardCommand() *cobra.Command {
	var (
		resumePaths         []string
		skillsFlag          string
		accomplishmentsFlag string
		resetFlag           bool
		yesFlag             bool
	)

	cmd := &cobra.Command{
		Use:   "onboard",
		Short: "Store resumes, skills, and accomplishments in the profile database",
		Long: `Read resume, skills, and accomplishments files, embed them, and store
them in the local profile database for use during the apply pipeline.

Use --reset to clear the existing profile database and inputs/ directory.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Handle --reset flag
			if resetFlag {
				if err := resetProfile(cfg, yesFlag, cmd); err != nil {
					return err
				}

				// If no other flags are provided, return after reset
				if len(resumePaths) == 0 && skillsFlag == "" && accomplishmentsFlag == "" {
					fmt.Fprintln(cmd.OutOrStdout(), "Profile reset. Run 'go-apply onboard --resume <path>' to re-onboard.")
					return nil
				}
				// Otherwise, continue to onboard with the provided flags
			}

			if len(resumePaths) == 0 && skillsFlag == "" && accomplishmentsFlag == "" {
				return fmt.Errorf("at least one of --resume, --skills, or --accomplishments is required")
			}
			if len(resumePaths) == 0 {
				return fmt.Errorf("resume is required")
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
			defer profileRepo.Close() //nolint:errcheck

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
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&resumePaths, "resume", nil, "Path to a resume file (repeatable)")
	cmd.Flags().StringVar(&skillsFlag, "skills", "", "Path to skills reference file")
	cmd.Flags().StringVar(&accomplishmentsFlag, "accomplishments", "", "Path to accomplishments file")
	cmd.Flags().BoolVar(&resetFlag, "reset", false, "Delete profile.db and inputs/ directory")
	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompt for --reset (required for non-interactive mode)")

	return cmd
}

// resetProfile deletes the profile database and inputs directory.
// If --yes is not set and stdin is a TTY, prompts the user for confirmation.
// If --yes is not set and stdin is not a TTY, returns an error.
func resetProfile(cfg *config.Config, confirmed bool, cmd *cobra.Command) error {
	// Check if stdin is a TTY
	isTTY := isatty.IsTerminal(os.Stdin.Fd())

	// If not confirmed and not a TTY, require --yes
	if !confirmed && !isTTY {
		return fmt.Errorf("--yes required for non-interactive reset")
	}

	// If not confirmed and is a TTY, prompt for confirmation
	if !confirmed && isTTY {
		fmt.Fprintf(cmd.OutOrStdout(), "Delete profile.db and inputs/ at %s? This cannot be undone. [y/N]: ", config.DataDir())
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("reset cancelled")
		}
		response := strings.TrimSpace(scanner.Text())
		if response != "y" && response != "Y" {
			fmt.Fprintln(cmd.OutOrStdout(), "Reset cancelled.")
			return nil
		}
	}

	// Delete profile.db
	dbPath := cfg.ResolveDBPath()
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete profile.db: %w", err)
	}

	// Delete WAL and SHM files (ignore if they don't exist)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	// Delete inputs directory
	inputsDir := filepath.Join(config.DataDir(), "inputs")
	if err := os.RemoveAll(inputsDir); err != nil {
		return fmt.Errorf("delete inputs directory: %w", err)
	}

	return nil
}

// labelFromPath returns the filename stem (no extension, no directory) for a resume path.
func labelFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	label := strings.TrimSuffix(base, ext)
	return label
}
