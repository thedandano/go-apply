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

// Service invokes pdftotext to extract plain text from PDF bytes.
type Service struct {
	lookPath func(string) (string, error)
	cmdFunc  func(context.Context, string, ...string) *exec.Cmd
	timeout  time.Duration
}

// New returns a Service configured with real exec helpers and a 10s timeout.
func New() *Service {
	return &Service{
		lookPath: exec.LookPath,
		cmdFunc:  exec.CommandContext,
		timeout:  10 * time.Second,
	}
}

// Extract invokes pdftotext on data and returns the extracted plain text.
// Returns "", nil for empty or nil input.
func (s *Service) Extract(data []byte) (string, error) {
	// Step 1: empty/nil input — callers treat empty result as signal to check renderer output.
	if len(data) == 0 {
		return "", nil
	}

	// Step 2: FR-009 — log attempt.
	slog.Info("extract.attempt", "binary", "pdftotext")

	// Step 3: locate pdftotext binary.
	path, err := s.lookPath("pdftotext")
	if err != nil {
		return "", fmt.Errorf("extractor: pdftotext not found — run go-apply doctor to diagnose missing dependencies: %w", err)
	}

	// Step 4: FR-009 — log invocation.
	slog.Info("extract.invoke")

	// Step 5: run subprocess — pdftotext reads stdin ("-") and writes to stdout ("-").
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	cmd := s.cmdFunc(ctx, path, "-", "-")
	cmd.Stdin = bytes.NewReader(data)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		sanitized := sanitizeStderr(stderr.Bytes())
		return "", fmt.Errorf("pdftotext: exit 1: %s: %w", sanitized, err)
	}

	// Step 6: return extracted text.
	return stdout.String(), nil
}

// sanitizeStderr caps raw stderr to 256 bytes and strips non-printable / high bytes.
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
