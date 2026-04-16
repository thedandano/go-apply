// Package cli provides Cobra commands for the go-apply CLI.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/mcpserver"
)

// NewServeCommand returns the cobra command for "go-apply serve".
// It starts an MCP stdio server that exposes pipeline tools for Claude Code integration.
func NewServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the MCP stdio server for Claude Code integration",
		RunE: func(_ *cobra.Command, _ []string) error {
			return mcpserver.Serve()
		},
	}
}
