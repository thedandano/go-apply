// Package onboardcheck provides a shared guard that verifies the user has
// completed onboarding before pipeline or tool operations can proceed.
package onboardcheck

import (
	"fmt"
	"strings"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
)

// CheckOnboarded verifies that the user is ready to run the pipeline.
// It checks two conditions:
//  1. The embedder is configured (base_url and model are set).
//  2. At least one resume file exists in the profile data directory.
func CheckOnboarded(cfg *config.Config, resumes port.ResumeRepository) error {
	if strings.TrimSpace(cfg.Embedder.BaseURL) == "" || strings.TrimSpace(cfg.Embedder.Model) == "" {
		return fmt.Errorf("embedder not configured — use update_config to set embedder.base_url and embedder.model before scoring")
	}
	files, err := resumes.ListResumes()
	if err != nil {
		return fmt.Errorf("check resumes: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no resumes found — use onboard_user to add your resume before scoring")
	}
	return nil
}
