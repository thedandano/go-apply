package extract

import "github.com/thedandano/go-apply/internal/port"

var _ port.Extractor = (*Service)(nil)

// Service is the identity extractor: text in, text out.
// A real PDF extractor (e.g. pdftotext) can be swapped in without changing callers.
type Service struct{}

func New() *Service { return &Service{} }

func (s *Service) Extract(text string) (string, error) { return text, nil }
