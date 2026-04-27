package extract_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/go-pdf/fpdf"

	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/extract"
)

var _ port.Extractor = extract.New()

// minimalPDF returns a valid single-page PDF with the given text embedded.
// Uses go-pdf/fpdf (already a project dep) to produce real PDF bytes.
func minimalPDF(text string) []byte {
	f := fpdf.New("P", "mm", "A4", "")
	f.AddPage()
	f.SetFont("Arial", "", 12)
	f.Cell(40, 10, text)
	var buf bytes.Buffer
	_ = f.Output(&buf)
	return buf.Bytes()
}

func TestExtract_EmptyBytes(t *testing.T) {
	svc := extract.New()
	out, err := svc.Extract(context.Background(), []byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("Extract of empty bytes must return empty, got: %q", out)
	}
}

func TestExtract_NilBytes(t *testing.T) {
	svc := extract.New()
	out, err := svc.Extract(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error for nil input: %v", err)
	}
	if out != "" {
		t.Errorf("Extract of nil must return empty, got: %q", out)
	}
}

// T007: normal PDF bytes → non-empty string, no error.
func TestExtract_ValidPDF_ReturnsNonEmptyText(t *testing.T) {
	svc := extract.New()
	data := minimalPDF("Hello World")
	out, err := svc.Extract(context.Background(), data)
	if err != nil {
		t.Fatalf("unexpected error for valid PDF: %v", err)
	}
	if len(out) == 0 {
		t.Error("Extract of valid PDF must return non-empty text")
	}
}

// T007: corrupt/non-PDF bytes → non-nil error.
func TestExtract_CorruptBytes_ReturnsError(t *testing.T) {
	svc := extract.New()
	corrupt := []byte("this is not a PDF")
	_, err := svc.Extract(context.Background(), corrupt)
	if err == nil {
		t.Error("expected error for corrupt/non-PDF bytes, got nil")
	}
}

// Input larger than maxPDFInputBytes must be rejected before parsing.
func TestExtract_OversizedInput_ReturnsError(t *testing.T) {
	svc := extract.New()
	// 11 MB — exceeds the 10 MB cap.
	oversized := make([]byte, 11*1024*1024)
	_, err := svc.Extract(context.Background(), oversized)
	if err == nil {
		t.Error("expected error for oversized input, got nil")
	}
}

// A PDF that causes the underlying library to panic must not crash the process.
func TestExtract_MalformedPDF_DoesNotPanic(_ *testing.T) {
	svc := extract.New()
	// Craft a byte slice that starts like a PDF header but is otherwise garbage,
	// maximizing the chance that the parser reaches an unexpected code path.
	malformed := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte{0xFF, 0x00}, 512)...)
	// We only care that Extract returns without panicking.
	_, _ = svc.Extract(context.Background(), malformed)
}
