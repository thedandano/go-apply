package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
}

func TestTextExtractorLoad(t *testing.T) {
	dir := t.TempDir()
	content := "Hello, world!\nThis is a test resume."
	path := filepath.Join(dir, "resume.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
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
	// Minimal DOCX document.xml with w:t elements
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

	want := "Hello World"
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Errorf("extractXMLText = %q, want it to contain %q", got, want)
	}
	if !strings.Contains(got, "Go Developer") {
		t.Errorf("extractXMLText = %q, want it to contain 'Go Developer'", got)
	}
}

func TestDOCXExtractor_extractXMLText_IgnoresNonNamespaced(t *testing.T) {
	// Elements named "t" but without a namespace should be ignored
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
