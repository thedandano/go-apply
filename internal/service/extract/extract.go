package extract

import (
	"bytes"
	"context"
	"fmt"
	"io"

	pdf "github.com/ledongthuc/pdf"

	"github.com/thedandano/go-apply/internal/port"
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
func (s *Service) Extract(_ context.Context, data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}

	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("extract: open pdf: %w", err)
	}
	tr, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract: get plain text: %w", err)
	}
	b, err := io.ReadAll(tr)
	if err != nil {
		return "", fmt.Errorf("extract: read text: %w", err)
	}
	return string(b), nil
}
