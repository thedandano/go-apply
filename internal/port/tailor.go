package port

import (
	"context"

	"github.com/thedandano/go-apply/internal/model"
)

// TailorOptions carries behavior-controlling limits for the tailor service.
// These values come from AppDefaults and are extracted by the CLI/MCP layer before
// calling TailorResume — the port package must not import internal/config.
type TailorOptions struct {
	MaxTier2BulletRewrites int
}

type TailorInput struct {
	Resume              model.ResumeFile
	ResumeText          string // pre-extracted by the pipeline before calling TailorResume
	JD                  model.JDData
	ScoreBefore         model.ScoreResult
	AccomplishmentsText string
	SkillsRefText       string
	Options             TailorOptions
}

type Tailor interface {
	TailorResume(ctx context.Context, input TailorInput) (model.TailorResult, error)
}
