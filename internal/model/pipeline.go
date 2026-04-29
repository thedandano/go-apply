package model

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
