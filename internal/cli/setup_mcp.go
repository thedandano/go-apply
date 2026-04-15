package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/agentconfig"
)

func newSetupMCPCommand() *cobra.Command {
	var agent string
	var remove bool

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Register or unregister go-apply as an MCP server in an AI agent's config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			registrar, err := agentconfig.NewRegistrar(agent)
			if err != nil {
				return err
			}

			if remove {
				result, err := registrar.Unregister("go-apply")
				if err != nil {
					return err
				}
				switch result.Action {
				case port.ActionRemoved:
					fmt.Fprintf(cmd.OutOrStdout(), "Removed go-apply MCP server from %s\n", result.ConfigPath)
				case port.ActionNotFound:
					fmt.Fprintf(cmd.OutOrStdout(), "go-apply MCP server not found in %s\n", result.ConfigPath)
				}
				return nil
			}

			execPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("resolve binary path: %w", err)
			}
			entry := port.MCPServerEntry{Command: execPath, Args: []string{"serve"}}
			result, err := registrar.Register("go-apply", entry)
			if err != nil {
				return err
			}
			switch result.Action {
			case port.ActionCreated:
				fmt.Fprintf(cmd.OutOrStdout(), "Created %s with go-apply MCP server\n", result.ConfigPath)
			case port.ActionAdded:
				fmt.Fprintf(cmd.OutOrStdout(), "Added go-apply MCP server to %s\n", result.ConfigPath)
			case port.ActionAlreadyRegistered:
				fmt.Fprintf(cmd.OutOrStdout(), "go-apply MCP server already registered in %s\n", result.ConfigPath)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&agent, "agent", "", "AI agent to configure (claude, openclaw, hermes)")
	cmd.Flags().BoolVar(&remove, "remove", false, "unregister go-apply from the agent's config")
	if err := cmd.MarkFlagRequired("agent"); err != nil {
		panic(err) // programming error — flag name must match
	}
	return cmd
}
