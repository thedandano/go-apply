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
	"github.com/thedandano/go-apply/internal/service/onboarding"
)

// NewOnboardCommand returns the cobra command for "go-apply onboard".
// It accepts one or more resume files plus optional skills and accomplishments paths.
// When --reset is set, it clears inputs/, skills.md, accomplishments.json, and accomplishments-*.md (legacy).
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
		Short: "Store resumes, skills, and accomplishments in the data directory",
		Long: `Read resume, skills, and accomplishments files, and write them to the data directory.
Resumes are written to inputs/, skills to skills.md, and accomplishments to accomplishments.json.

Use --reset to clear inputs/, skills.md, accomplishments.json, and accomplishments-*.md (legacy).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Handle --reset flag
			if resetFlag {
				if err := resetProfile(yesFlag, cmd); err != nil {
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

			log := slog.Default()
			svc := onboarding.New(config.DataDir(), log)

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
	cmd.Flags().BoolVar(&resetFlag, "reset", false, "Delete inputs/, skills.md, accomplishments.json, and accomplishments-*.md (legacy) from the data directory")
	cmd.Flags().BoolVar(&yesFlag, "yes", false, "Skip confirmation prompt for --reset (required for non-interactive mode)")

	return cmd
}

// resetProfile deletes the onboarding data: inputs/, skills.md, accomplishments.json, and accomplishments-*.md (legacy).
// If --yes is not set and stdin is a TTY, prompts the user for confirmation.
// If --yes is not set and stdin is not a TTY, returns an error.
func resetProfile(confirmed bool, cmd *cobra.Command) error {
	// Check if stdin is a TTY.
	isTTY := isatty.IsTerminal(os.Stdin.Fd())

	if !confirmed && !isTTY {
		return fmt.Errorf("--yes required for non-interactive reset")
	}

	if !confirmed && isTTY {
		fmt.Fprintf(cmd.OutOrStdout(), "Delete inputs/, skills.md, accomplishments.json, and accomplishments-*.md at %s? This cannot be undone. [y/N]: ", config.DataDir())
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

	dataDir := config.DataDir()

	// Delete inputs/ directory (resumes).
	inputsDir := filepath.Join(dataDir, "inputs")
	if err := os.RemoveAll(inputsDir); err != nil {
		return fmt.Errorf("delete inputs directory: %w", err)
	}

	// Delete skills.md (non-existence is not an error).
	skillsPath := filepath.Join(dataDir, "skills.md")
	if err := os.Remove(skillsPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete skills.md: %w", err)
	}

	// Delete all accomplishments-*.md files (legacy).
	matches, err := filepath.Glob(filepath.Join(dataDir, "accomplishments-*.md"))
	if err != nil {
		return fmt.Errorf("glob accomplishments files: %w", err)
	}
	for _, match := range matches {
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %s: %w", match, err)
		}
	}

	// Delete accomplishments.json (if it exists).
	accomplishmentsPath := filepath.Join(dataDir, "accomplishments.json")
	if err := os.Remove(accomplishmentsPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete accomplishments.json: %w", err)
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
