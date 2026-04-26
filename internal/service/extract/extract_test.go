package extract_test

import (
	"context"
	"testing"

	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/extract"
)

var _ port.Extractor = extract.New()

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
