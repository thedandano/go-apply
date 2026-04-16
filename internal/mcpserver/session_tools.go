package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	mcppres "github.com/thedandano/go-apply/internal/presenter/mcp"
	"github.com/thedandano/go-apply/internal/service/pipeline"
)

// sessions is the process-lifetime session store shared by all multi-turn handlers.
var sessions = NewSessionStore()

// HandleLoadJD is the exported handler for the "load_jd" MCP tool.
// In production, deps and session store come from the process-level instances.
func HandleLoadJD(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleLoadJDWithConfig(ctx, req, nil)
}

// HandleLoadJDWithConfig is the full handler with optional injected deps (for tests).
// When deps is nil, loadDeps() is called to get live dependencies.
func HandleLoadJDWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig) *mcp.CallToolResult {
	jdURL := req.GetString("jd_url", "")
	jdRawText := req.GetString("jd_raw_text", "")
	if (jdURL != "") == (jdRawText != "") {
		return envelopeResult(stageErrorEnvelope("", "load_jd", "invalid_input",
			"exactly one of jd_url or jd_raw_text is required", false))
	}

	urlOrText := jdURL
	isText := false
	if jdRawText != "" {
		urlOrText = jdRawText
		isText = true
	}

	if deps == nil {
		_, liveDeps, err := loadDeps()
		if err != nil {
			return envelopeResult(stageErrorEnvelope("", "load_jd", "config_error", err.Error(), true))
		}
		deps = &liveDeps
	}

	pres := mcppres.New()
	deps.Presenter = pres
	pl := pipeline.NewApplyPipeline(deps)

	jdText, err := pl.AcquireJD(ctx, urlOrText, isText)
	if err != nil {
		slog.ErrorContext(ctx, "load_jd: acquire failed", "error", err)
		return envelopeResult(stageErrorEnvelope("", "fetch", "fetch_failed", err.Error(), true))
	}

	sess := sessions.Create(jdText)
	sess.URL = jdURL
	sess.IsText = isText

	type loadJDData struct {
		JDText string `json:"jd_text"`
	}
	return envelopeResult(okEnvelope(sess.ID, "extract_keywords", loadJDData{JDText: jdText}))
}

// HandleSubmitKeywords is the exported handler for the "submit_keywords" MCP tool.
func HandleSubmitKeywords(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleSubmitKeywordsWithConfig(ctx, req, nil, nil)
}

// HandleSubmitKeywordsWithConfig is the full handler with optional injected deps (for tests).
func HandleSubmitKeywordsWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig, cfg *config.Config) *mcp.CallToolResult {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "submit_keywords", "missing_session", "session_id is required", false))
	}
	jdJSONStr := req.GetString("jd_json", "")
	if jdJSONStr == "" {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "missing_jd", "jd_json is required", false))
	}

	sess := sessions.Get(sessionID)
	if sess == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State != stateLoaded {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "invalid_state",
			fmt.Sprintf("expected state %q, got %q — call load_jd first", stateLoaded, sess.State), false))
	}

	var jd model.JDData
	if err := json.Unmarshal([]byte(jdJSONStr), &jd); err != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "invalid_jd",
			fmt.Sprintf("jd_json parse failed: %v", err), false))
	}
	if len(jd.Required) == 0 && len(jd.Preferred) == 0 &&
		strings.TrimSpace(jd.Title) == "" && strings.TrimSpace(jd.Company) == "" {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "jd_empty",
			"jd_json contains no extractable keywords — provide at least title, company, or required skills", false))
	}

	if deps == nil {
		liveCfg, liveDeps, err := loadDeps()
		if err != nil {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "config_error", err.Error(), true))
		}
		deps = &liveDeps
		if cfg == nil {
			cfg = liveCfg
		}
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	pres := mcppres.New()
	deps.Presenter = pres
	pl := pipeline.NewApplyPipeline(deps)

	scored, err := pl.ScoreResumes(ctx, &jd, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "submit_keywords: score failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "score", "score_failed", err.Error(), false))
	}

	sess.JD = jd
	sess.ScoreResult = scored
	sess.State = stateScored

	nextAction := NextActionFromScore(scored.BestScore)

	type submitKeywordsData struct {
		Scores     map[string]model.ScoreResult `json:"scores"`
		BestResume string                       `json:"best_resume"`
		BestScore  float64                      `json:"best_score"`
	}
	return envelopeResult(okEnvelope(sessionID, nextAction, submitKeywordsData{
		Scores:     scored.Scores,
		BestResume: scored.BestLabel,
		BestScore:  scored.BestScore,
	}))
}

// HandleFinalize is the exported handler for the "finalize" MCP tool.
func HandleFinalize(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleFinalizeWithConfig(ctx, req, nil)
}

// HandleFinalizeWithConfig is the full handler with optional injected deps (for tests).
func HandleFinalizeWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig) *mcp.CallToolResult {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "finalize", "missing_session", "session_id is required", false))
	}
	coverLetter := req.GetString("cover_letter", "")

	sess := sessions.Get(sessionID)
	if sess == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "finalize", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State < stateScored {
		return envelopeResult(stageErrorEnvelope(sessionID, "finalize", "invalid_state",
			fmt.Sprintf("session must be scored before finalize; current state: %q", sess.State), false))
	}

	// Persist the application record for URL-based flows.
	if sess.URL != "" {
		if deps == nil {
			_, liveDeps, err := loadDeps()
			if err == nil {
				deps = &liveDeps
			}
		}
		if deps != nil {
			pres := mcppres.New()
			deps.Presenter = pres
			rec := &model.ApplicationRecord{
				URL:         sess.URL,
				RawText:     sess.JDText,
				JD:          sess.JD,
				CoverLetter: coverLetter,
			}
			if bestScore, ok := sess.ScoreResult.Scores[sess.ScoreResult.BestLabel]; ok {
				rec.Score = &bestScore
				rec.ResumeLabel = sess.ScoreResult.BestLabel
			}
			if putErr := deps.AppRepo.Put(rec); putErr != nil {
				slog.WarnContext(ctx, "finalize: failed to persist record", "session_id", sessionID, "error", putErr)
			}
		}
	}

	sess.State = stateFinalized

	type finalizeData struct {
		BestResume  string  `json:"best_resume"`
		BestScore   float64 `json:"best_score"`
		CoverLetter string  `json:"cover_letter,omitempty"`
	}
	return envelopeResult(okEnvelope(sessionID, "", finalizeData{
		BestResume:  sess.ScoreResult.BestLabel,
		BestScore:   sess.ScoreResult.BestScore,
		CoverLetter: coverLetter,
	}))
}

// NextActionFromScore returns the recommended next action based on the best resume score.
// Exported for testing.
func NextActionFromScore(score float64) string {
	switch {
	case score < 0.40:
		return "advise_skip"
	case score < 0.70:
		return "tailor_t1"
	default:
		return "cover_letter"
	}
}
