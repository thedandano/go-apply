package extract

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestExtract_LookPathMissing_ReturnsDescriptiveError(t *testing.T) {
	svc := &Service{
		lookPath: func(string) (string, error) { return "", exec.ErrNotFound },
	}
	_, err := svc.Extract(context.Background(), []byte("pdf bytes"))
	if err == nil {
		t.Fatal("expected error when lookPath returns ErrNotFound, got nil")
	}
	if msg := err.Error(); !strings.Contains(msg, "go-apply doctor") {
		t.Errorf("error message must mention \"go-apply doctor\", got: %q", msg)
	}
}

func TestExtract_LookPathFound_SubprocessSuccess(t *testing.T) {
	svc := &Service{
		lookPath: func(string) (string, error) { return "/usr/bin/cat", nil },
		cmdFunc: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "cat")
		},
		timeout: 5 * time.Second,
	}
	input := []byte("hello extracted text")
	got, err := svc.Extract(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != string(input) {
		t.Errorf("Extract must return subprocess stdout: got %q, want %q", got, string(input))
	}
}

func TestExtract_SubprocessFailure_StderrSanitized(t *testing.T) {
	svc := &Service{
		lookPath: func(string) (string, error) { return "/usr/bin/cat", nil },
		cmdFunc: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c",
				"python3 -c \"import sys; sys.stderr.buffer.write(bytes(range(256))*2); sys.exit(1)\"")
		},
		timeout: 5 * time.Second,
	}
	_, err := svc.Extract(context.Background(), []byte("pdf bytes"))
	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
	msg := err.Error()
	if len(msg) > 512 {
		t.Errorf("error message suspiciously long (%d bytes); stderr portion should be capped at 256 bytes", len(msg))
	}
	for i, b := range []byte(msg) {
		if b < 0x20 {
			t.Errorf("non-printable byte 0x%02x at position %d in error message", b, i)
		}
		if b > 0x7e {
			t.Errorf("high byte 0x%02x at position %d in error message", b, i)
		}
	}
}

func TestExtract_ContextTimeout_KillsSubprocess(t *testing.T) {
	svc := &Service{
		lookPath: func(string) (string, error) { return "/bin/sleep", nil },
		cmdFunc: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sleep", "10")
		},
		timeout: 50 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.Extract(context.Background(), []byte("some data"))
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from context timeout, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Error("Extract did not return within 2 seconds after context timeout")
	}
}

func TestExtract_CallerContextCancelled_KillsSubprocess(t *testing.T) {
	svc := &Service{
		lookPath: func(string) (string, error) { return "/bin/sleep", nil },
		cmdFunc: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sleep", "10")
		},
		timeout: 30 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := svc.Extract(ctx, []byte("some data"))
		done <- err
	}()

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from caller context cancellation, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Error("Extract did not return within 2 seconds after caller context cancel")
	}
}

func TestExtract_LogsAttemptAndInvocation(t *testing.T) {
	origLogger := slog.Default()
	h := &testLogHandler{}
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(origLogger)

	svc := &Service{
		lookPath: func(string) (string, error) { return "/usr/bin/cat", nil },
		cmdFunc: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "cat")
		},
		timeout: 5 * time.Second,
	}
	_, err := svc.Extract(context.Background(), []byte("data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !h.hasMsg("extract.attempt") {
		t.Error("expected slog record with message \"extract.attempt\" to be emitted")
	}
	if !h.hasMsg("extract.invoke") {
		t.Error("expected slog record with message \"extract.invoke\" to be emitted")
	}
}

// TestExtract_InputExceedsMaxPDFBytes_ReturnsError verifies that the 10 MB input cap
// is enforced before the subprocess is invoked.
func TestExtract_InputExceedsMaxPDFBytes_ReturnsError(t *testing.T) {
	svc := &Service{
		lookPath: func(string) (string, error) { return "/usr/bin/cat", nil },
		timeout:  5 * time.Second,
	}
	data := make([]byte, maxPDFBytes+1)
	_, err := svc.Extract(context.Background(), data)
	if err == nil {
		t.Fatal("expected error for input exceeding 10 MB, got nil")
	}
}

// TestExtract_InputAtExactMaxPDFBytes_DoesNotError verifies that a payload exactly
// at the cap boundary is accepted (boundary-value: cap is exclusive).
func TestExtract_InputAtExactMaxPDFBytes_DoesNotError(t *testing.T) {
	svc := &Service{
		lookPath: func(string) (string, error) { return "/usr/bin/cat", nil },
		cmdFunc: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "cat")
		},
		timeout: 5 * time.Second,
	}
	data := make([]byte, maxPDFBytes)
	_, err := svc.Extract(context.Background(), data)
	if err != nil {
		t.Fatalf("payload at exact cap must be accepted: %v", err)
	}
}

// TestExtract_OutputExceedsMaxOutputBytes_ReturnsError verifies that the streaming
// stdout cap is enforced and Extract returns an error when pdftotext emits too much.
func TestExtract_OutputExceedsMaxOutputBytes_ReturnsError(t *testing.T) {
	svc := &Service{
		lookPath: func(string) (string, error) { return "/usr/bin/cat", nil },
		cmdFunc: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			// 11 * 1 MB = 11 MB, reliably exceeds the 10 MB output cap.
			return exec.CommandContext(ctx, "dd", "if=/dev/zero", "bs=1048576", "count=11")
		},
		timeout: 30 * time.Second,
	}
	_, err := svc.Extract(context.Background(), []byte("pdf"))
	if err == nil {
		t.Fatal("expected error for output exceeding 10 MB, got nil")
	}
}

type testLogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *testLogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *testLogHandler) Handle(_ context.Context, r slog.Record) error { //nolint:gocritic // slog.Handler interface requires value receiver
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *testLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *testLogHandler) WithGroup(_ string) slog.Handler      { return h }
func (h *testLogHandler) hasMsg(msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.records {
		if h.records[i].Message == msg {
			return true
		}
	}
	return false
}
