package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewVersionCommand returns a command that prints the build version.
func NewVersionCommand(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the go-apply version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "go-apply version %s\n", version)
		},
	}
}
