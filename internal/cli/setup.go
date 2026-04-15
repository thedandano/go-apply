package cli

import "github.com/spf13/cobra"

// NewSetupCommand returns the cobra command for "go-apply setup".
func NewSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Configure go-apply integrations",
	}
	cmd.AddCommand(newSetupMCPCommand())
	return cmd
}
