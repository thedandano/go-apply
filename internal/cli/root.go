package cli

import "github.com/spf13/cobra"

// NewRootCommand returns the root cobra command for go-apply.
func NewRootCommand(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "go-apply",
		Short: "AI-powered job application assistant",
	}
	cmd.AddCommand(NewApplyCommand())
	cmd.AddCommand(NewServeCommand())
	cmd.AddCommand(NewConfigCommand())
	cmd.AddCommand(NewOnboardCommand())
	cmd.AddCommand(NewVersionCommand(version))
	cmd.AddCommand(NewSetupCommand())
	return cmd
}
