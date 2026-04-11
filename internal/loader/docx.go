package loader

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/thedandano/go-apply/internal/port"
)

// DOCXExtractor handles .docx files using stdlib zip+xml — no external license required.
type DOCXExtractor struct{}

var _ port.DocumentLoader = (*DOCXExtractor)(nil)

func (d *DOCXExtractor) Supports(ext string) bool {
	return strings.EqualFold(ext, ".docx")
}

func (d *DOCXExtractor) Load(path string) (string, error) {
	r, err := zip.OpenReader(path) // #nosec G304 -- caller-supplied document path
	if err != nil {
		return "", fmt.Errorf("open docx %s: %w", path, err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("open word/document.xml in %s: %w", path, err)
		}
		text, xmlErr := extractXMLText(rc)
		rc.Close()
		return text, xmlErr
	}
	return "", fmt.Errorf("word/document.xml not found in %s", path)
}

// extractXMLText collects all <w:t> text content from a DOCX document.xml.
func extractXMLText(r io.Reader) (string, error) {
	var sb strings.Builder
	dec := xml.NewDecoder(r)
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse document.xml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			inText = t.Name.Local == "t" && t.Name.Space != ""
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}
	return sb.String(), nil
}
