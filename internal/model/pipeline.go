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

type RiskWarning struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type DegradedStep struct {
	Step   string `json:"step"`
	Reason string `json:"reason"`
}

type PipelineResult struct {
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`

	JD         JDData                 `json:"jd"`
	Scores     map[string]ScoreResult `json:"scores"`
	BestScore  float64                `json:"best_score"`
	BestResume string                 `json:"best_resume"`
	Keywords   struct {
		Required  []string `json:"required"`
		Preferred []string `json:"preferred"`
	} `json:"keywords"`

	Cascade     *TailorResult     `json:"cascade,omitempty"`
	CoverLetter CoverLetterResult `json:"cover_letter"`
	Warnings    []RiskWarning     `json:"warnings"`

	DegradedSteps  []DegradedStep `json:"degraded_steps,omitempty"`
	AnalysisTimeMS int64          `json:"analysis_time_ms"`
}

func NewPipelineResult() *PipelineResult {
	return &PipelineResult{
		Status: "success",
		Scores: make(map[string]ScoreResult),
	}
}

func (r *PipelineResult) AddDegraded(step, reason string) {
	r.DegradedSteps = append(r.DegradedSteps, DegradedStep{Step: step, Reason: reason})
}

func (r *PipelineResult) SetTiming(start time.Time) {
	r.AnalysisTimeMS = time.Since(start).Milliseconds()
}
