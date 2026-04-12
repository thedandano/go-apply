package cli

import (
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
)

// NewRootCommand returns the root cobra command for go-apply.
// defaults is injected so sub-commands can use application configuration
// without constructing it themselves.
func NewRootCommand(defaults *config.AppDefaults) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "go-apply",
		Short: "AI-powered job application assistant",
	}
	cmd.AddCommand(newApplyCommand(defaults))
	cmd.AddCommand(newServeCommand(defaults))
	return cmd
}
