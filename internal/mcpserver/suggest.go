package mcpserver

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/logger"
	mcppres "github.com/thedandano/go-apply/internal/presenter/mcp"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// HandleSuggestTailoring is the exported handler for the "suggest_tailoring" MCP tool.
func HandleSuggestTailoring(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleSuggestTailoringWithConfig(ctx, req, nil)
}

// HandleSuggestTailoringWithConfig is the full handler with optional injected deps (for tests).
func HandleSuggestTailoringWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig) *mcp.CallToolResult {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "suggest_tailoring", "missing_session", "session_id is required", false))
	}
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "suggest_tailoring"),
		slog.String("session_id", sessionID),
	)

	sess := sessions.Get(sessionID)
	if sess == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "suggest_tailoring", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State < stateScored {
		return envelopeResult(stageErrorEnvelope(sessionID, "suggest_tailoring", "invalid_state",
			"session must be scored before suggest_tailoring; call submit_keywords first", false))
	}

	if deps == nil {
		_, liveDeps, err := loadDeps()
		if err != nil {
			return envelopeResult(stageErrorEnvelope(sessionID, "suggest_tailoring", "config_error", err.Error(), true))
		}
		deps = &liveDeps
	}

	pres := mcppres.New()
	deps.Presenter = pres
	pl := pipeline.NewApplyPipeline(deps)

	logger.Banner(ctx, slog.Default(), "Augment", "Profile Retrieval")
	suggestions, retrievalMode, err := pl.SuggestTailoring(ctx, &sess.JD, sess.ScoreResult)
	if err != nil {
		slog.ErrorContext(ctx, "suggest_tailoring: failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "suggest_tailoring", "retrieval_failed", err.Error(), false))
	}

	// Serialize suggestions as keyword-keyed arrays for the orchestrator.
	type matchEntry struct {
		Source     string  `json:"source"`
		Text       string  `json:"text"`
		Similarity float32 `json:"similarity"`
	}
	type keywordMatches struct {
		Keyword string       `json:"keyword"`
		Matches []matchEntry `json:"matches"`
	}

	buildList := func(keywords []string) []keywordMatches {
		result := make([]keywordMatches, 0, len(keywords))
		for _, kw := range keywords {
			chunks := suggestions[kw]
			matches := make([]matchEntry, 0, len(chunks))
			for _, c := range chunks {
				matches = append(matches, matchEntry{Source: c.SourceDoc, Text: c.Text, Similarity: c.Similarity})
			}
			result = append(result, keywordMatches{Keyword: kw, Matches: matches})
		}
		return result
	}

	type suggestData struct {
		Required      []keywordMatches `json:"required"`
		Preferred     []keywordMatches `json:"preferred"`
		RetrievalMode string           `json:"retrieval_mode"`
	}

	resultData := suggestData{
		Required:      buildList(sess.JD.Required),
		Preferred:     buildList(sess.JD.Preferred),
		RetrievalMode: retrievalMode,
	}

	resultBytes, _ := json.Marshal(resultData)
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "suggest_tailoring"),
		slog.String("session_id", sessionID),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(resultBytes)),
		logger.PayloadAttr("result", string(resultBytes), logger.Verbose()),
	)
	return envelopeResult(okEnvelope(sessionID, NextActionFromScore(sess.ScoreResult.BestScore), resultData))
}
