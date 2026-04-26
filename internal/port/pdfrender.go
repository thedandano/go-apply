package port

import "github.com/thedandano/go-apply/internal/model"

// PDFRenderer converts a SectionMap into PDF bytes suitable for ATS extraction.
// For ATS-safe plain text used by the tailor pipeline, see Renderer.
// The concrete implementation lives in internal/service/pdfrender/.
type PDFRenderer interface {
	RenderPDF(sections *model.SectionMap) ([]byte, error)
}
