package loader

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/thedandano/go-apply/internal/port"
)

// PDFExtractor handles .pdf files.
type PDFExtractor struct{}

var _ port.DocumentLoader = (*PDFExtractor)(nil)

func (p *PDFExtractor) Supports(ext string) bool {
	return strings.EqualFold(ext, ".pdf")
}

func (p *PDFExtractor) Load(path string) (string, error) {
	f, r, err := pdf.Open(path) // #nosec G304 -- caller-supplied document path
	if err != nil {
		return "", fmt.Errorf("open pdf %s: %w", path, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("extract text from pdf %s: %w", path, err)
	}
	if _, err := buf.ReadFrom(b); err != nil {
		return "", fmt.Errorf("read pdf text %s: %w", path, err)
	}
	return buf.String(), nil
}
