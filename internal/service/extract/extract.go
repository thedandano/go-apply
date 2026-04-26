package extract

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/thedandano/go-apply/internal/port"
)

var _ port.Extractor = (*Service)(nil)

// maxPDFBytes is the maximum accepted PDF input size (10 MB).
const maxPDFBytes = 10 * 1024 * 1024

// maxOutputBytes is the maximum accepted pdftotext stdout size (10 MB).
const maxOutputBytes = 10 * 1024 * 1024

// Service invokes pdftotext to extract plain text from PDF bytes.
type Service struct {
	lookPath func(string) (string, error)
	cmdFunc  func(context.Context, string, ...string) *exec.Cmd
	timeout  time.Duration
}

// New returns a Service configured with real exec helpers and a 30s timeout.
func New() *Service {
	return &Service{
		lookPath: exec.LookPath,
		cmdFunc:  exec.CommandContext,
		timeout:  30 * time.Second,
	}
}

// Extract invokes pdftotext on data and returns the extracted plain text.
// Returns "", nil for empty or nil input.
// The context is used to propagate caller cancellation to the subprocess.
func (s *Service) Extract(ctx context.Context, data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	if len(data) > maxPDFBytes {
		return "", fmt.Errorf("extract: PDF size %d bytes exceeds limit of %d bytes", len(data), maxPDFBytes)
	}

	slog.Info("extract.attempt", "binary", "pdftotext")

	path, err := s.lookPath("pdftotext")
	if err != nil {
		return "", fmt.Errorf("extractor: pdftotext not found — run go-apply doctor to diagnose missing dependencies: %w", err)
	}

	slog.Info("extract.invoke", "path", path)

	// Derive a timeout sub-context from the caller context so pdftotext is
	// cancelled when the caller disconnects or times out.
	tctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	cmd := s.cmdFunc(tctx, path, "-", "-")
	cmd.Stdin = bytes.NewReader(data)
	// cappedWriter caps stdout mid-stream so pathological PDFs cannot exhaust RAM
	// before cmd.Run() returns.
	var stdout bytes.Buffer
	cw := &cappedWriter{w: &stdout, remaining: maxOutputBytes}
	cmd.Stdout = cw
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		sanitized := sanitizeStderr(stderr.Bytes())
		return "", fmt.Errorf("pdftotext: exit 1: %s: %w", sanitized, err)
	}

	if cw.exceeded {
		return "", fmt.Errorf("extract: pdftotext output exceeded maximum size of %d bytes", maxOutputBytes)
	}

	return stdout.String(), nil
}

// cappedWriter is an io.Writer that forwards bytes to w up to remaining bytes,
// then silently discards writes and marks exceeded=true.
type cappedWriter struct {
	w         *bytes.Buffer
	remaining int
	exceeded  bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.exceeded {
		return len(p), nil
	}
	if len(p) > c.remaining {
		c.exceeded = true
		c.w.Write(p[:c.remaining]) //nolint:errcheck // buffer error irrelevant once cap exceeded; cw.exceeded checked post-run
		c.remaining = 0
		return len(p), nil
	}
	n, err := c.w.Write(p)
	c.remaining -= n
	return n, err
}

// sanitizeStderr caps raw stderr to 256 bytes and strips all bytes outside
// the ASCII printable range [0x20, 0x7e]. Tab and newline are excluded.
func sanitizeStderr(raw []byte) string {
	if len(raw) > 256 {
		raw = raw[:256]
	}
	var b strings.Builder
	for _, c := range raw {
		if c >= 0x20 && c <= 0x7e {
			b.WriteByte(c)
		}
	}
	return b.String()
}
