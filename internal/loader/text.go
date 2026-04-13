package loader

import (
	"fmt"
	"os"
	"strings"

	"github.com/thedandano/go-apply/internal/port"
)

// TextExtractor handles .txt, .text, .md, and .markdown files.
type TextExtractor struct{}

var _ port.DocumentLoader = (*TextExtractor)(nil)

func (t *TextExtractor) Supports(ext string) bool {
	switch strings.ToLower(ext) {
	case ".txt", ".text", ".md", ".markdown":
		return true
	}
	return false
}

func (t *TextExtractor) Load(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is caller-supplied resume/reference file, not user input to a web endpoint
	if err != nil {
		return "", fmt.Errorf("read text file %s: %w", path, err)
	}
	return string(data), nil
}
