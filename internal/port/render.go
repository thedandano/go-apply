package port

import "github.com/thedandano/go-apply/internal/model"

// Renderer converts a SectionMap into ATS-safe plain text.
// The concrete implementation lives in internal/service/render/.
type Renderer interface {
	Render(sections *model.SectionMap) (string, error)
}
