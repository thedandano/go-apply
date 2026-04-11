package loader

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/thedandano/go-apply/internal/port"
)

// Dispatcher satisfies port.DocumentLoader by routing to the appropriate
// format-specific extractor based on file extension.
type Dispatcher struct {
	extractors []port.DocumentLoader
}

// New returns a Dispatcher pre-loaded with all supported format extractors.
func New() *Dispatcher {
	return &Dispatcher{
		extractors: []port.DocumentLoader{
			&DOCXExtractor{},
			&PDFExtractor{},
			&TextExtractor{},
		},
	}
}

var _ port.DocumentLoader = (*Dispatcher)(nil)

// Supports returns true if any registered extractor supports the given extension.
func (d *Dispatcher) Supports(ext string) bool {
	ext = strings.ToLower(ext)
	for _, e := range d.extractors {
		if e.Supports(ext) {
			return true
		}
	}
	return false
}

// Load extracts plain text from the file at path.
// Returns an error if no extractor supports the file extension.
func (d *Dispatcher) Load(path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range d.extractors {
		if e.Supports(ext) {
			return e.Load(path)
		}
	}
	return "", fmt.Errorf("unsupported file extension %q for %s", ext, path)
}
