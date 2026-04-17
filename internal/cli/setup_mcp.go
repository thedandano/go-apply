package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/agentconfig"
)

// allAgents is the ordered list of all known agent names for --agent all.
var allAgents = []string{"claude", "openclaw", "hermes"}

func newSetupMCPCommand() *cobra.Command {
	var (
		agent    string
		remove   bool
		override bool
	)

	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Register or unregister go-apply as an MCP server in an AI agent's config",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if agent == "all" {
				return runSetupMCPAll(cmd, remove, override)
			}
			return runSetupMCPSingle(cmd, agent, remove, override)
		},
	}

	cmd.Flags().StringVar(&agent, "agent", "", "AI agent to configure (claude, openclaw, hermes, all)")
	cmd.Flags().BoolVar(&remove, "remove", false, "unregister go-apply from the agent's config")
	cmd.Flags().BoolVar(&override, "override", false, "overwrite existing registration")
	cmd.Flags().BoolVar(&override, "force", false, "overwrite existing registration (alias for --override)")
	if err := cmd.MarkFlagRequired("agent"); err != nil {
		panic(err) // programming error — flag name must match
	}
	return cmd
}

// runSetupMCPSingle handles a single named agent.
func runSetupMCPSingle(cmd *cobra.Command, agent string, remove, override bool) error {
	registrar, err := agentconfig.NewRegistrar(agent)
	if err != nil {
		return err
	}

	if remove {
		result, err := registrar.Unregister("go-apply")
		if err != nil {
			return err
		}
		printRemoveResult(cmd, result)
		return nil
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}
	entry := port.MCPServerEntry{Command: execPath, Args: []string{"serve"}}

	if override {
		result, err := registrar.RegisterForce("go-apply", entry)
		if err != nil {
			return err
		}
		printRegisterResult(cmd, result)
		return nil
	}

	result, err := registrar.Register("go-apply", entry)
	if err != nil {
		return err
	}

	// On non-TTY or not already-registered, print status and return.
	if result.Action != port.ActionAlreadyRegistered || !isatty.IsTerminal(os.Stdin.Fd()) {
		printRegisterResult(cmd, result)
		return nil
	}

	// TTY: prompt to overwrite.
	fmt.Fprintf(cmd.OutOrStdout(), "go-apply is already registered with %s. Overwrite? [y/N]: ", agent)
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		fmt.Fprintln(cmd.OutOrStdout(), "Skipped.")
		return nil
	}
	resp := strings.TrimSpace(scanner.Text())
	if resp != "y" && resp != "Y" {
		fmt.Fprintln(cmd.OutOrStdout(), "Skipped.")
		return nil
	}
	forceResult, err := registrar.RegisterForce("go-apply", entry)
	if err != nil {
		return err
	}
	printRegisterResult(cmd, forceResult)
	return nil
}

// runSetupMCPAll iterates over all known agents, collecting errors without aborting.
func runSetupMCPAll(cmd *cobra.Command, remove, override bool) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}
	entry := port.MCPServerEntry{Command: execPath, Args: []string{"serve"}}

	var errs []error
	for _, a := range allAgents {
		registrar, regErr := agentconfig.NewRegistrar(a)
		if regErr != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: error (%v)\n", a, regErr)
			errs = append(errs, regErr)
			continue
		}

		if remove {
			result, rmErr := registrar.Unregister("go-apply")
			if rmErr != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: error (%v)\n", a, rmErr)
				errs = append(errs, rmErr)
				continue
			}
			printRemoveResultPrefixed(cmd, a, result)
			continue
		}

		if override {
			result, forceErr := registrar.RegisterForce("go-apply", entry)
			if forceErr != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: error (%v)\n", a, forceErr)
				errs = append(errs, forceErr)
				continue
			}
			printRegisterResultPrefixed(cmd, a, result)
			continue
		}

		result, regErr2 := registrar.Register("go-apply", entry)
		if regErr2 != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: error (%v)\n", a, regErr2)
			errs = append(errs, regErr2)
			continue
		}
		printRegisterResultPrefixed(cmd, a, result)
	}

	return errors.Join(errs...)
}

// printRegisterResult prints a single-agent registration status line.
func printRegisterResult(cmd *cobra.Command, result port.RegistrationResult) {
	switch result.Action {
	case port.ActionCreated:
		fmt.Fprintf(cmd.OutOrStdout(), "Created %s with go-apply MCP server\n", result.ConfigPath)
	case port.ActionAdded:
		fmt.Fprintf(cmd.OutOrStdout(), "Added go-apply MCP server to %s\n", result.ConfigPath)
	case port.ActionAlreadyRegistered:
		fmt.Fprintf(cmd.OutOrStdout(), "go-apply MCP server already registered in %s\n", result.ConfigPath)
	}
}

// printRemoveResult prints a single-agent removal status line.
func printRemoveResult(cmd *cobra.Command, result port.RegistrationResult) {
	switch result.Action {
	case port.ActionRemoved:
		fmt.Fprintf(cmd.OutOrStdout(), "Removed go-apply MCP server from %s\n", result.ConfigPath)
	case port.ActionNotFound:
		fmt.Fprintf(cmd.OutOrStdout(), "go-apply MCP server not found in %s\n", result.ConfigPath)
	}
}

// printRegisterResultPrefixed prints an "agent: status" line for --agent all.
func printRegisterResultPrefixed(cmd *cobra.Command, agent string, result port.RegistrationResult) {
	switch result.Action {
	case port.ActionCreated, port.ActionAdded:
		fmt.Fprintf(cmd.OutOrStdout(), "%s: registered\n", agent)
	case port.ActionAlreadyRegistered:
		fmt.Fprintf(cmd.OutOrStdout(), "%s: already registered (use --override to overwrite)\n", agent)
	}
}

// printRemoveResultPrefixed prints an "agent: status" line for --remove --agent all.
func printRemoveResultPrefixed(cmd *cobra.Command, agent string, result port.RegistrationResult) {
	switch result.Action {
	case port.ActionRemoved:
		fmt.Fprintf(cmd.OutOrStdout(), "%s: removed\n", agent)
	case port.ActionNotFound:
		fmt.Fprintf(cmd.OutOrStdout(), "%s: not found\n", agent)
	}
}
