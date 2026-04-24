package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// ExtractKeywordsInput holds the inputs for keyword extraction.
type ExtractKeywordsInput struct {
	JDText string
}

// PlanT1Input holds the inputs for tier-1 tailoring planning.
type PlanT1Input struct {
	JDData     model.JDData
	ResumeText string
	SkillsRef  string // contents of the skills reference document
}

// PlanT1Output holds the result of tier-1 tailoring planning.
type PlanT1Output struct {
	SkillAdds []string // skills to inject into the skills section
}

// PlanT2Input holds the inputs for tier-2 tailoring planning.
type PlanT2Input struct {
	JDData          model.JDData
	ResumeText      string
	Accomplishments string
}

// BulletRewrite pairs an original bullet text with its replacement.
type BulletRewrite struct {
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
}

// PlanT2Output holds the result of tier-2 tailoring planning.
type PlanT2Output struct {
	Rewrites []BulletRewrite
}

// CoverLetterInput holds the inputs for cover letter generation.
type CoverLetterInput struct {
	JDData        model.JDData
	ResumeText    string
	CandidateName string
}

// Orchestrator is the decision-point interface for LLM-driven pipeline steps.
// In CLI/TUI mode an in-process LLM implements this interface.
// In MCP mode the MCP host (Claude) acts as orchestrator and this interface
// is not used — the pipeline receives pre-computed results from the MCP tools.
type Orchestrator interface {
	ExtractKeywords(ctx context.Context, input ExtractKeywordsInput) (model.JDData, error)
	PlanT1(ctx context.Context, input *PlanT1Input) (PlanT1Output, error)
	PlanT2(ctx context.Context, input *PlanT2Input) (PlanT2Output, error)
	GenerateCoverLetter(ctx context.Context, input *CoverLetterInput) (string, error)
	ParseSections(ctx context.Context, rawResume string) (model.SectionMap, error)
}
