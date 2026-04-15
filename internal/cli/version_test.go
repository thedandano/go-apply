package cli_test

import (
	"bytes"
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
)

func TestVersionCommand_PrintsVersion(t *testing.T) {
	root := cli.NewRootCommand("1.2.3")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"version"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	if got != "go-apply version 1.2.3\n" {
		t.Errorf("got %q, want %q", got, "go-apply version 1.2.3\n")
	}
}

func TestVersionCommand_DevDefault(t *testing.T) {
	root := cli.NewRootCommand("dev")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"version"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	if got != "go-apply version dev\n" {
		t.Errorf("got %q, want %q", got, "go-apply version dev\n")
	}
}
