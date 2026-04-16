package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

func NewLogsCommand() *cobra.Command {
	var lines int
	var follow bool
	var logDir string

	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View recent go-apply logs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if logDir == "" {
				logDir = logDirPath()
			}
			return runLogs(cmd, logDir, lines, follow)
		},
	}
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "number of recent lines to show")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "watch for new log lines")
	cmd.Flags().StringVar(&logDir, "log-dir", "", "override log directory (for testing)")
	_ = cmd.Flags().MarkHidden("log-dir")
	return cmd
}

func logDirPath() string {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "go-apply", "logs")
}

func mostRecentLogFile(logDir string) (string, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return "", fmt.Errorf("read log dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && len(e.Name()) > 9 &&
			e.Name()[:9] == "go-apply-" && filepath.Ext(e.Name()) == ".log" {
			files = append(files, filepath.Join(logDir, e.Name()))
		}
	}
	if len(files) == 0 {
		return "", errors.New("no log files found")
	}
	sort.Strings(files)
	return files[len(files)-1], nil
}

func tailLines(r io.Reader, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	ring := make([]string, n)
	pos := 0
	count := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		ring[pos%n] = scanner.Text()
		pos++
		count++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if count <= n {
		return ring[:count], nil
	}
	start := pos % n
	result := make([]string, n)
	copy(result, ring[start:])
	copy(result[n-start:], ring[:start])
	return result, nil
}

func runLogs(cmd *cobra.Command, logDir string, n int, follow bool) error {
	path, err := mostRecentLogFile(logDir)
	if err != nil {
		return err
	}

	f, err := os.Open(path) //#nosec G304 -- path is constructed from XDG_STATE_HOME + fixed suffix
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	tail, err := tailLines(f, n)
	if err != nil {
		return fmt.Errorf("read log: %w", err)
	}
	for _, line := range tail {
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}

	if !follow {
		return nil
	}

	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("seek: %w", err)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-ticker.C:
			fi, err := f.Stat()
			if err != nil || fi.Size() <= offset {
				continue
			}
			if _, err := f.Seek(offset, io.SeekStart); err != nil {
				continue
			}
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				fmt.Fprintln(cmd.OutOrStdout(), scanner.Text())
			}
			offset, _ = f.Seek(0, io.SeekCurrent)
		}
	}
}
