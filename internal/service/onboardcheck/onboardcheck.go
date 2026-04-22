// Package onboardcheck provides a shared guard that verifies the user has
// completed onboarding before pipeline or tool operations can proceed.
package onboardcheck

import (
	"fmt"

	"github.com/thedandano/go-apply/internal/port"
)

// CheckOnboarded verifies that the user is ready to run the pipeline.
// It checks that at least one resume file exists in the profile data directory.
func CheckOnboarded(resumes port.ResumeRepository) error {
	files, err := resumes.ListResumes()
	if err != nil {
		return fmt.Errorf("check resumes: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no resumes found — use onboard_user to add your resume before scoring")
	}
	return nil
}
