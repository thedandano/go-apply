package extract_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/extract"
)

var _ port.Extractor = extract.New()

func TestExtract_Identity(t *testing.T) {
	svc := extract.New()
	input := "Some resume text with keywords: Go, Python, Kubernetes"
	out, err := svc.Extract(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != input {
		t.Errorf("Extract must be identity: got %q, want %q", out, input)
	}
}

func TestExtract_EmptyString(t *testing.T) {
	svc := extract.New()
	out, err := svc.Extract("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("Extract of empty string must return empty, got: %q", out)
	}
}
