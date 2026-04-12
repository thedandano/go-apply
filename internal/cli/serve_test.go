package cli_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
	"github.com/thedandano/go-apply/internal/config"
)

func TestNewServeCommand_Registered(t *testing.T) {
	defaults, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	root := cli.NewRootCommand(defaults)
	cmds := root.Commands()

	for _, cmd := range cmds {
		if cmd.Use == "serve" {
			return
		}
	}
	t.Errorf("serve subcommand not found; registered commands: %v", func() []string {
		names := make([]string, len(cmds))
		for i, c := range cmds {
			names[i] = c.Use
		}
		return names
	}())
}
