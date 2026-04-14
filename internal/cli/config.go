package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
)

// NewConfigCommand returns the cobra command for "go-apply config".
// Subcommands: set, get, show.
func NewConfigCommand() *cobra.Command {
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
			key, value := args[0], args[1]
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if err := cfg.SetField(key, value); err != nil {
				return err
			}
			if err := cfg.Save(); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("%s = %s\n", key, value)
			return nil
		},
	}
}

func newConfigGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Get a config field value by dot-notation key",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			key := args[0]
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			value, err := cfg.GetField(key)
			if err != nil {
				return err
			}
			fmt.Printf("%s = %s\n", key, value)
			return nil
		},
	}
}

func newConfigShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show all config fields (API keys redacted)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			for _, key := range config.AllKeys() {
				value, err := cfg.GetField(key)
				if err != nil {
					return err
				}
				if config.IsAPIKey(key) && value != "" {
					value = "***"
				}
				fmt.Printf("%s = %s\n", key, value)
			}
			return nil
		},
	}
}
