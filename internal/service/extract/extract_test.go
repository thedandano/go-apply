package extract_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/extract"
)

var _ port.Extractor = extract.New()

func TestExtract_EmptyString(t *testing.T) {
	svc := extract.New()
	out, err := svc.Extract([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("Extract of empty string must return empty, got: %q", out)
	}
}
