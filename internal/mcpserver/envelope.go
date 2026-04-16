package mcpserver

import (
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/model"
)

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

// envelopeResult marshals env to JSON and wraps it in an MCP text result.
func envelopeResult(env *Envelope) *mcp.CallToolResult {
	data, _ := json.Marshal(env)
	return mcp.NewToolResultText(string(data))
}

// okEnvelope builds a success envelope.
func okEnvelope(sessionID, nextAction string, data any) *Envelope {
	return &Envelope{
		SessionID:  sessionID,
		Status:     "ok",
		NextAction: nextAction,
		Data:       data,
	}
}

// stageErrorEnvelope builds a structured error envelope for a named pipeline stage.
func stageErrorEnvelope(sessionID, stage, code, message string, retriable bool) *Envelope {
	return &Envelope{
		SessionID: sessionID,
		Status:    "error",
		Error: &StageError{
			Stage:     stage,
			Code:      code,
			Message:   message,
			Retriable: retriable,
		},
	}
}
