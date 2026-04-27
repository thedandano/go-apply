package extract

import (
	"bytes"
	"context"
	"fmt"
	"io"

	// ledongthuc/pdf has no tagged releases; the pseudo-version pin is intentional.
	pdf "github.com/ledongthuc/pdf"

	"github.com/thedandano/go-apply/internal/port"
)

const (
	maxPDFInputBytes  = 10 * 1024 * 1024 // 10 MB — guard against oversized uploads
	maxExtractedBytes = 16 * 1024 * 1024 // 16 MB — guard against pathologically dense PDFs
)

var _ port.Extractor = (*Service)(nil)

// Service extracts plain text from PDF bytes using the ledongthuc/pdf library.
type Service struct{}

// New returns a Service ready to extract text from PDF bytes.
func New() *Service {
	return &Service{}
}

// Extract converts PDF bytes to plain text.
// Returns ("", nil) for empty or nil input; the caller is responsible for
// treating empty output as an extraction failure.
// ctx is accepted for interface compliance and caller-cancellation awareness,
// but the ledongthuc/pdf library does not support context cancellation.
func (s *Service) Extract(ctx context.Context, data []byte) (out string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("extract: pdf panic: %v", r)
		}
	}()

	if len(data) == 0 {
		return "", nil
	}
	if len(data) > maxPDFInputBytes {
		return "", fmt.Errorf("extract: input too large (%d bytes, max %d)", len(data), maxPDFInputBytes)
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}

	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("extract: open pdf: %w", err)
	}
	tr, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract: get plain text: %w", err)
	}
	b, err := io.ReadAll(io.LimitReader(tr, maxExtractedBytes))
	if err != nil {
		return "", fmt.Errorf("extract: read text: %w", err)
	}
	return string(b), nil
}
