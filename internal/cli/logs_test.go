package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/thedandano/go-apply/internal/cli"
)

func executeLogsCmd(t *testing.T, logDir string, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "root"}
	cmd := cli.NewLogsCommand()
	root.AddCommand(cmd)
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	cmd.SetOut(buf)
	root.SetArgs(append([]string{"logs", "--log-dir", logDir}, args...))
	err := root.Execute()
	return buf.String(), err
}

func writeFixture(t *testing.T, dir, name string, lineCount int) {
	t.Helper()
	var sb strings.Builder
	for i := range lineCount {
		fmt.Fprintf(&sb, "line %d\n", i+1)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(sb.String()), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestLogsCommand_ShowsLastNLines(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "go-apply-2025-01-01.log", 20)
	out, err := executeLogsCmd(t, dir, "-n", "5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines, got %d: %q", len(lines), out)
	}
	if lines[0] != "line 16" {
		t.Errorf("expected first line to be 'line 16', got %q", lines[0])
	}
}

func TestLogsCommand_DefaultLines(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "go-apply-2025-01-01.log", 150)
	out, err := executeLogsCmd(t, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 100 {
		t.Errorf("expected 100 lines, got %d", len(lines))
	}
}

func TestLogsCommand_NoLogFiles(t *testing.T) {
	dir := t.TempDir()
	_, err := executeLogsCmd(t, dir)
	if err == nil || !strings.Contains(err.Error(), "no log files found") {
		t.Errorf("expected 'no log files found' error, got: %v", err)
	}
}

func TestLogsCommand_PicksMostRecentFile(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "go-apply-2025-01-01.log", 3)
	if err := os.WriteFile(filepath.Join(dir, "go-apply-2025-06-15.log"), []byte("newer line\n"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := executeLogsCmd(t, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "newer line") {
		t.Errorf("expected output from newer file, got: %q", out)
	}
}

func TestLogsCommand_FollowOutputsNewLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "go-apply-2025-01-01.log")
	if err := os.WriteFile(logPath, []byte("existing line\n"), 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	root := &cobra.Command{Use: "root"}
	cmd := cli.NewLogsCommand()
	root.AddCommand(cmd)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	root.SetArgs([]string{"logs", "--log-dir", dir, "--follow"})

	done := make(chan error, 1)
	go func() {
		cmd.SetContext(ctx)
		done <- root.Execute()
	}()

	time.Sleep(200 * time.Millisecond)
	f, _ := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0600)
	fmt.Fprintln(f, "new line appended")
	f.Close()

	time.Sleep(700 * time.Millisecond)
	cancel()
	<-done

	if !strings.Contains(buf.String(), "new line appended") {
		t.Errorf("expected 'new line appended' in output, got: %q", buf.String())
	}
}

func TestLogsCommand_FollowExitsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go-apply-2025-01-01.log"), []byte("line\n"), 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	root := &cobra.Command{Use: "root"}
	cmd := cli.NewLogsCommand()
	root.AddCommand(cmd)
	cmd.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"logs", "--log-dir", dir, "--follow"})

	done := make(chan error, 1)
	go func() {
		cmd.SetContext(ctx)
		done <- root.Execute()
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("command did not exit within 1s after context cancel")
	}
}

func TestLogsCommand_NegativeLines(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "go-apply-2025-01-01.log", 10)
	out, err := executeLogsCmd(t, dir, "-n", "0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty output for -n 0, got: %q", out)
	}
}
