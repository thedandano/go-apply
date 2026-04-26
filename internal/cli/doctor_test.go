package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/cli"
)

func TestDoctor_PdftotextPresent_PrintsOK(t *testing.T) {
	cmd := cli.NewDoctorCommandWithLookPath(func(_ string) (string, error) {
		return "/usr/bin/pdftotext", nil
	})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "[OK]") {
		t.Errorf("output %q missing [OK]", got)
	}
	if !strings.Contains(got, "pdftotext") {
		t.Errorf("output %q missing 'pdftotext'", got)
	}
}

func TestDoctor_PdftotextMissing_PrintsMISSING(t *testing.T) {
	cmd := cli.NewDoctorCommandWithLookPath(func(_ string) (string, error) {
		return "", errors.New("not found")
	})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(new(bytes.Buffer)) // suppress cobra error output

	_ = cmd.Execute() // ignore error; tested separately

	got := buf.String()
	if !strings.Contains(got, "[MISSING]") {
		t.Errorf("output %q missing [MISSING]", got)
	}
	if !strings.Contains(got, "poppler") {
		t.Errorf("output %q missing 'poppler' installation hint", got)
	}
}

func TestDoctor_PdftotextMissing_ExitCodeOne(t *testing.T) {
	cmd := cli.NewDoctorCommandWithLookPath(func(_ string) (string, error) {
		return "", errors.New("not found")
	})
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer)) // suppress cobra error output

	err := cmd.Execute()
	if err == nil {
		t.Error("expected non-nil error (exit code 1) when pdftotext is missing")
	}
}
