package model

import "time"

// No score threshold constants here — values come from AppDefaults injected at runtime.

type StepStartedEvent struct {
	StepID string `json:"step_id"`
	Label  string `json:"label"`
}

type StepCompletedEvent struct {
	StepID    string `json:"step_id"`
	Label     string `json:"label"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type StepFailedEvent struct {
	StepID string `json:"step_id"`
	Label  string `json:"label"`
	Err    string `json:"error"`
}

// Warning severity levels used in RiskWarning.Severity.
const (
	SeverityWarn  = "warn"  // non-fatal degradation; processing continues
	SeverityError = "error" // invalid input rejected before processing
)

type RiskWarning struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type PipelineResult struct {
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	JDText  string `json:"jd_text,omitempty"` // raw JD text; returned so Claude can reason over it in MCP mode

	JD         JDData                 `json:"jd"`
	Scores     map[string]ScoreResult `json:"scores"`
	BestScore  float64                `json:"best_score"`
	BestResume string                 `json:"best_resume"`
	Keywords   struct {
		Required  []string `json:"required"`
		Preferred []string `json:"preferred"`
	} `json:"keywords"`

	Warnings []RiskWarning `json:"warnings"`

	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

func NewPipelineResult() *PipelineResult {
	return &PipelineResult{
		Scores: make(map[string]ScoreResult),
	}
}
