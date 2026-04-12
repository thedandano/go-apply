package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	loaderPkg "github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/service/onboarding"
)

// newOnboardCommand returns the `onboard` cobra command.
func newOnboardCommand(_ *config.AppDefaults) *cobra.Command {
	var (
		resumePaths     []string
		label           string
		skillsPath      string
		accomplishments string
	)

	cmd := &cobra.Command{
		Use:   "onboard",
		Short: "Onboard resume, skills, and accomplishments into the profile database",
		Long: `onboard indexes resumes and reference documents into the local sqlite-vec
profile database. Once indexed, the apply command can augment resumes with
semantically relevant profile content.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runOnboard(cmd.Context(), resumePaths, label, skillsPath, accomplishments)
		},
	}

	cmd.Flags().StringArrayVar(&resumePaths, "resume", nil, "Resume file path (repeatable)")
	cmd.Flags().StringVar(&label, "label", "", "Label for the resume (default: filename without extension)")
	cmd.Flags().StringVar(&skillsPath, "skills", "", "Skills reference document path")
	cmd.Flags().StringVar(&accomplishments, "accomplishments", "", "Accomplishments document path")

	return cmd
}

func runOnboard(ctx context.Context, resumePaths []string, label, skillsPath, accomplishmentsPath string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	defaults, err := config.LoadDefaults()
	if err != nil {
		return fmt.Errorf("load defaults: %w", err)
	}

	embedderClient := newEmbedderClient(cfg, defaults)

	profileRepo, err := newSQLiteProfile(cfg, defaults)
	if err != nil {
		return fmt.Errorf("open profile db: %w", err)
	}

	docLoader := loaderPkg.New()
	dataDir := config.DataDir()

	svc := onboarding.New(profileRepo, embedderClient, dataDir)

	input := onboarding.OnboardInput{
		Resumes: make(map[string]onboarding.OnboardFile),
	}

	for _, path := range resumePaths {
		resolvedLabel := label
		if resolvedLabel == "" {
			base := filepath.Base(path)
			ext := filepath.Ext(base)
			resolvedLabel = strings.TrimSuffix(base, ext)
		}

		if _, exists := input.Resumes[resolvedLabel]; exists {
			return fmt.Errorf("duplicate resume label %q: use --label to assign unique labels", resolvedLabel)
		}

		text, loadErr := docLoader.Load(path)
		if loadErr != nil {
			return fmt.Errorf("load resume %s: %w", path, loadErr)
		}

		input.Resumes[resolvedLabel] = onboarding.OnboardFile{
			Label:     resolvedLabel,
			PlainText: text,
			OrigPath:  path,
			Format:    filepath.Ext(path),
		}
	}

	if skillsPath != "" {
		text, loadErr := docLoader.Load(skillsPath)
		if loadErr != nil {
			return fmt.Errorf("load skills %s: %w", skillsPath, loadErr)
		}
		input.SkillsText = text
	}

	if accomplishmentsPath != "" {
		text, loadErr := docLoader.Load(accomplishmentsPath)
		if loadErr != nil {
			return fmt.Errorf("load accomplishments %s: %w", accomplishmentsPath, loadErr)
		}
		input.AccomplishmentsText = text
	}

	result, err := svc.Run(ctx, input)
	if err != nil {
		return fmt.Errorf("onboard: %w", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
