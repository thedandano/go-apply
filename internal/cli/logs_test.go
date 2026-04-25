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

// T004: TestLogsCommand_RawFileUnchangedAfterDisplay verifies that the logs
// command is read-only: the fixture file's bytes are identical before and after
// the command runs.
func TestLogsCommand_RawFileUnchangedAfterDisplay(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "go-apply-2025-01-01.log")
	content := []byte(`2026-04-25 10:30:58 INFO mcp tool result tool=submit_tailor_t2 status=ok result="{\"score\":75}"` + "\n")
	if err := os.WriteFile(logPath, content, 0600); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read before: %v", err)
	}

	if _, err := executeLogsCmd(t, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	after, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}

	if !bytes.Equal(before, after) {
		t.Errorf("file was modified by the logs command\nbefore: %q\nafter:  %q", before, after)
	}
}

// T005: TestLogsCommand_ConsistentFormattingAcrossEntries verifies that two
// log lines each containing a different JSON-valued field are both rendered
// with the same indentation style: 2-space label indent and 4-space JSON body indent.
func TestLogsCommand_ConsistentFormattingAcrossEntries(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "go-apply-2025-01-01.log")
	content := `2026-04-25 10:00:00 INFO action one result="{\"score\":75}"` + "\n" +
		`2026-04-25 10:00:01 WARN action two payload="{\"items\":[1,2,3]}"` + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	out, err := executeLogsCmd(t, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, label := range []string{"  result:", "  payload:"} {
		if !strings.Contains(out, label) {
			t.Errorf("expected %q in output, got:\n%s", label, out)
		}
	}

	// Both JSON blocks must open with a 4-space-indented brace.
	// MarshalIndent(parsed, "    ", "  ") places the opening { at the prefix level.
	if strings.Count(out, "    {") < 2 {
		t.Errorf("expected at least 2 occurrences of '    {' (4-space indent), got:\n%s", out)
	}
}

// T005a: TestLogsCommand_FollowPrettyPrintsJSON verifies that --follow mode
// also pretty-prints JSON-valued fields as new lines are appended to the file.
func TestLogsCommand_FollowPrettyPrintsJSON(t *testing.T) {
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
	fmt.Fprintln(f, `2026-04-25 10:00:00 INFO scored result="{\"score\":99}"`)
	f.Close()

	time.Sleep(700 * time.Millisecond)
	cancel()
	<-done

	if !strings.Contains(buf.String(), "  result:") {
		t.Errorf("expected '  result:' in follow output, got: %q", buf.String())
	}
}
