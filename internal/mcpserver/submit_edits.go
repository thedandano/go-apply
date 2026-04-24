package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/tailor"
)

// HandleSubmitEdits is the exported handler for the "submit_edits" MCP tool.
func HandleSubmitEdits(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleSubmitEditsWithConfig(ctx, req, nil)
}

// HandleSubmitEditsWithConfig is the full handler with optional injected deps (for tests).
func HandleSubmitEditsWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig) *mcp.CallToolResult {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "submit_edits", "missing_session", "session_id is required", false))
	}

	editsStr := req.GetString("edits", "")
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "submit_edits"),
		slog.String("session_id", sessionID),
		logger.PayloadAttr("edits", editsStr, logger.Verbose()),
	)

	if editsStr == "" {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_edits", "missing_edits", "edits is required", false))
	}

	var edits []port.Edit
	if err := json.Unmarshal([]byte(editsStr), &edits); err != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_edits", "invalid_edits",
			fmt.Sprintf("edits parse failed: %v", err), false))
	}
	if len(edits) == 0 {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_edits", "empty_edits",
			"edits must contain at least one entry", false))
	}

	sess := sessions.Get(sessionID)
	if sess == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_edits", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State != stateScored && sess.State != stateT1Applied && sess.State != stateT2Applied {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_edits", "invalid_state",
			fmt.Sprintf("expected state %q, %q, or %q, got %q",
				stateScored, stateT1Applied, stateT2Applied, sess.State), false))
	}

	if deps == nil {
		_, liveDeps, err := loadDeps()
		if err != nil {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_edits", "config_error", err.Error(), true))
		}
		deps = &liveDeps
	}

	sections, err := deps.Resumes.LoadSections(sess.ScoreResult.BestLabel)
	if err != nil {
		if errors.Is(err, model.ErrSectionsMissing) {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_edits", "sections_missing",
				"no sections found for this resume — re-onboard with sections field", false))
		}
		slog.ErrorContext(ctx, "submit_edits: load sections failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_edits", "load_sections_failed", err.Error(), false))
	}

	svc := tailor.New(nil, deps.Defaults, slog.Default())
	editResult, err := svc.ApplyEdits(ctx, sections, edits)
	if err != nil {
		slog.ErrorContext(ctx, "submit_edits: apply edits failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_edits", "apply_edits_failed", err.Error(), false))
	}

	resultBytes, _ := json.Marshal(editResult)
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "submit_edits"),
		slog.String("session_id", sessionID),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(resultBytes)),
		logger.PayloadAttr("result", string(resultBytes), logger.Verbose()),
	)
	return envelopeResult(okEnvelope(sessionID, "", editResult))
}
