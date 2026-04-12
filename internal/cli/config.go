package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
)

// newConfigCommand returns the `config` cobra command with subcommands.
func newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage go-apply configuration",
	}

	cmd.AddCommand(newConfigSetCommand())
	cmd.AddCommand(newConfigGetCommand())
	cmd.AddCommand(newConfigShowCommand())

	return cmd
}

func newConfigSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config field by dot-notation key",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if err := cfg.SetField(args[0], args[1]); err != nil {
				return err
			}
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("config %s = %s\n", args[0], args[1])
			return nil
		},
	}
}

func newConfigGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a config field value by dot-notation key (API keys shown as [redacted])",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			val, err := cfg.GetField(args[0])
			if err != nil {
				return err
			}
			key := args[0]
			if isAPIKey(key) && val != "" {
				val = "[redacted]"
			}
			fmt.Println(val)
			return nil
		},
	}
}

func newConfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print all config as JSON with API keys redacted",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			// Copy config and redact API keys
			redacted := *cfg
			if redacted.Orchestrator.APIKey != "" {
				redacted.Orchestrator.APIKey = "[redacted]"
			}
			if redacted.Embedder.APIKey != "" {
				redacted.Embedder.APIKey = "[redacted]"
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(redacted)
		},
	}
}

// isAPIKey returns true if the config key refers to an API key field.
func isAPIKey(key string) bool {
	return key == "orchestrator.api_key" || key == "embedder.api_key"
}
