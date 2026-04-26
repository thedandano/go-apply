//go:build integration

package mcpserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/service/extract"
	"github.com/thedandano/go-apply/internal/service/pdfrender"
)

func TestHandlePreviewATSExtraction_HonestLoop_RealPdftotext(t *testing.T) {
	_, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not on PATH — run with real binary")
	}

	cfg := stubApplyConfigWithTier4Sections()
	pdfSvc, err := pdfrender.New()
	if err != nil {
		t.Fatalf("pdfrender.New(): %v", err)
	}
	extractSvc := extract.New()
	cfg.PDFRenderer = pdfSvc
	cfg.Extractor = extractSvc

	sessionID := buildScoredSession(t, &cfg)

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("preview_ats_extraction not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "ok" {
		t.Errorf("status = %v, want ok — full: %s", env["status"], text)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data in response")
	}
	constructedText, _ := data["constructed_text"].(string)
	if constructedText == "" {
		t.Errorf("constructed_text must be non-empty, got: %q", constructedText)
	}

	if !contains(constructedText, "Alice") {
		t.Errorf("constructed_text missing candidate name from ContactInfo, got:\n%s", constructedText)
	}
}

func TestHandlePreviewATSExtraction_PdftotextMissing_ErrorReferencesDoctorCmd(t *testing.T) {
	cfg := stubApplyConfigWithTier4Sections()
	cfg.PDFRenderer = &stubPDFRenderer{failRender: false}
	cfg.Extractor = &stubbedExtractorWithDoctorMsg{failExtract: true}

	sessionID := buildScoredSession(t, &cfg)

	previewReq := callToolRequest("preview_ats_extraction", map[string]any{"session_id": sessionID})
	result := mcpserver.HandlePreviewATSExtractionWithConfig(context.Background(), &previewReq, &cfg)
	text := extractText(t, result)

	var env map[string]any
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		t.Fatalf("preview_ats_extraction not JSON: %v — raw: %s", err, text)
	}
	if env["status"] != "error" {
		t.Errorf("status = %v, want error — full: %s", env["status"], text)
	}
	if !contains(text, "go-apply doctor") {
		t.Errorf("error message must reference 'go-apply doctor', got: %s", text)
	}
}

type stubbedExtractorWithDoctorMsg struct {
	failExtract bool
}

func (s *stubbedExtractorWithDoctorMsg) Extract(_ []byte) (string, error) {
	if s.failExtract {
		return "", fmt.Errorf("extractor: pdftotext not found — run go-apply doctor to diagnose missing dependencies")
	}
	return "extracted text", nil
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
