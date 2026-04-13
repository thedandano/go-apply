package loader

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
)

func TestDispatcherSupports(t *testing.T) {
	d := New()
	supported := []string{".pdf", ".docx", ".txt", ".md", ".markdown", ".text"}
	for _, ext := range supported {
		if !d.Supports(ext) {
			t.Errorf("Supports(%q) = false, want true", ext)
		}
	}

	unsupported := []string{".exe", ".csv"}
	for _, ext := range unsupported {
		if d.Supports(ext) {
			t.Errorf("Supports(%q) = true, want false", ext)
		}
	}
}

func TestDispatcherLoadUnsupported(t *testing.T) {
	d := New()
	_, err := d.Load("file.exe")
	if err == nil {
		t.Fatal("expected error for unsupported extension, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error %q does not contain 'unsupported'", err.Error())
	}
	if !strings.Contains(err.Error(), "file.exe") {
		t.Errorf("error %q does not contain file path", err.Error())
	}
}

func TestTextExtractorLoad(t *testing.T) {
	dir := t.TempDir()
	content := "Hello, world!\nThis is a test resume."
	path := filepath.Join(dir, "resume.txt")
	if err := os.WriteFile(path, []byte(content), config.FilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ext := &TextExtractor{}
	got, err := ext.Load(path)
	if err != nil {
		t.Fatalf("Load(%q): %v", path, err)
	}
	if got != content {
		t.Errorf("Load(%q) = %q, want %q", path, got, content)
	}
}

func TestTextExtractorLoad_MissingFile(t *testing.T) {
	ext := &TextExtractor{}
	_, err := ext.Load("/nonexistent/path/file.txt")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestDOCXExtractor_extractXMLText(t *testing.T) {
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r>
        <w:t>Hello</w:t>
      </w:r>
      <w:r>
        <w:t> World</w:t>
      </w:r>
    </w:p>
    <w:p>
      <w:r>
        <w:t>Go Developer</w:t>
      </w:r>
    </w:p>
  </w:body>
</w:document>`

	r := strings.NewReader(xmlContent)
	got, err := extractXMLText(r)
	if err != nil {
		t.Fatalf("extractXMLText: %v", err)
	}

	// Paragraphs must be separated by newlines — not fused together.
	want := "Hello World\nGo Developer\n"
	if got != want {
		t.Errorf("extractXMLText = %q, want %q", got, want)
	}
}

func TestDOCXExtractor_extractXMLText_IgnoresNonNamespaced(t *testing.T) {
	xmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<document>
  <body>
    <t>This should be ignored</t>
    <w:r xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
      <w:t>This should be included</w:t>
    </w:r>
  </body>
</document>`

	r := strings.NewReader(xmlContent)
	got, err := extractXMLText(r)
	if err != nil {
		t.Fatalf("extractXMLText: %v", err)
	}
	if strings.Contains(got, "should be ignored") {
		t.Errorf("extractXMLText included non-namespaced <t> content: %q", got)
	}
	if !strings.Contains(got, "This should be included") {
		t.Errorf("extractXMLText = %q, want it to contain 'This should be included'", got)
	}
}

// buildMinimalDOCX creates an in-memory DOCX zip with the given document.xml content.
func buildMinimalDOCX(t *testing.T, docXML string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("zip.Create: %v", err)
	}
	if _, err := f.Write([]byte(docXML)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	path := filepath.Join(t.TempDir(), "test.docx")
	if err := os.WriteFile(path, buf.Bytes(), config.FilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestDOCXExtractorLoad(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body><w:p><w:r><w:t>Test content</w:t></w:r></w:p></w:body>
</w:document>`

	path := buildMinimalDOCX(t, docXML)
	d := &DOCXExtractor{}
	got, err := d.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.Contains(got, "Test content") {
		t.Errorf("Load = %q, want it to contain 'Test content'", got)
	}
}

func TestDOCXExtractorLoad_MissingDocumentXML(t *testing.T) {
	// Build a zip that has no word/document.xml
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("word/styles.xml")
	_, _ = f.Write([]byte("<styles/>"))
	_ = zw.Close()

	path := filepath.Join(t.TempDir(), "empty.docx")
	if err := os.WriteFile(path, buf.Bytes(), config.FilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	d := &DOCXExtractor{}
	_, err := d.Load(path)
	if err == nil {
		t.Fatal("expected error for missing word/document.xml, got nil")
	}
	if !strings.Contains(err.Error(), "word/document.xml not found") {
		t.Errorf("error %q does not mention missing document.xml", err.Error())
	}
}

func TestDOCXExtractorLoad_InvalidZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.docx")
	if err := os.WriteFile(path, []byte("not a zip"), config.FilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	d := &DOCXExtractor{}
	_, err := d.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid zip, got nil")
	}
}

func TestPDFExtractorLoad_MissingFile(t *testing.T) {
	p := &PDFExtractor{}
	_, err := p.Load("/nonexistent/path/resume.pdf")
	if err == nil {
		t.Fatal("expected error for missing PDF, got nil")
	}
	if !strings.Contains(err.Error(), "/nonexistent/path/resume.pdf") {
		t.Errorf("error %q does not contain file path", err.Error())
	}
}
