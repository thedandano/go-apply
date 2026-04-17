package cli_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
)

func TestRootCommand_LogLevelFlagRegistered(t *testing.T) {
	root := cli.NewRootCommand("test")
	f := root.PersistentFlags().Lookup("log-level")
	if f == nil {
		t.Fatal("--log-level flag not registered on root command")
	}
	if f.Value.Type() != "string" {
		t.Errorf("--log-level flag type = %q, want %q", f.Value.Type(), "string")
	}
}

func TestRootCommand_DebugFlagRegistered(t *testing.T) {
	root := cli.NewRootCommand("test")
	f := root.PersistentFlags().Lookup("debug")
	if f == nil {
		t.Fatal("--debug flag not registered on root command")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("--debug flag type = %q, want %q", f.Value.Type(), "bool")
	}
	// -v shorthand
	sh := root.PersistentFlags().ShorthandLookup("v")
	if sh == nil {
		t.Fatal("-v shorthand not registered for --debug flag")
	}
}

func TestRootCommand_TraceFlagRegistered(t *testing.T) {
	root := cli.NewRootCommand("test")
	f := root.PersistentFlags().Lookup("trace")
	if f == nil {
		t.Fatal("--trace flag not registered on root command")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("--trace flag type = %q, want %q", f.Value.Type(), "bool")
	}
}
