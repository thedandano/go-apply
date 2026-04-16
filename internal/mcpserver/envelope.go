package mcpserver

import "github.com/thedandano/go-apply/internal/model"

// Envelope is the structured response returned by every multi-turn MCP tool.
// Status values: "ok", "needs_input", "error".
// NextAction tells the host which tool to call next.
type Envelope struct {
	SessionID  string              `json:"session_id,omitempty"`
	Status     string              `json:"status"`
	NextAction string              `json:"next_action,omitempty"`
	Data       any                 `json:"data,omitempty"`
	Error      *StageError         `json:"error,omitempty"`
	Warnings   []model.RiskWarning `json:"warnings,omitempty"`
}

// StageError describes a failure at a named pipeline stage.
type StageError struct {
	Stage     string `json:"stage"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retriable bool   `json:"retriable"`
}
