package cli

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

// DoctorCmd checks that required external binaries are available.
type DoctorCmd struct {
	lookPath func(string) (string, error)
}

// NewDoctorCommand returns a doctor subcommand using exec.LookPath.
func NewDoctorCommand() *cobra.Command {
	return NewDoctorCommandWithLookPath(exec.LookPath)
}

// NewDoctorCommandWithLookPath returns a doctor subcommand with an injected lookPath.
func NewDoctorCommandWithLookPath(lp func(string) (string, error)) *cobra.Command {
	d := &DoctorCmd{lookPath: lp}
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check required external dependencies",
		RunE:  d.run,
	}
}

func (d *DoctorCmd) run(cmd *cobra.Command, _ []string) error {
	path, err := d.lookPath("pdftotext")
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(),
			"[MISSING] pdftotext — install poppler-utils (Linux) or: brew install poppler (macOS)\n")
		return fmt.Errorf("pdftotext not found: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "[OK] pdftotext — %s\n", path)
	return nil
}
