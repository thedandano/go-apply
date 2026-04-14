package cli

import "github.com/spf13/cobra"

// NewRootCommand returns the root cobra command for go-apply.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "go-apply",
		Short: "AI-powered job application assistant",
	}
	cmd.AddCommand(NewApplyCommand())
	return cmd
}
