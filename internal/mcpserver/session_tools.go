package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/model"
	mcppres "github.com/thedandano/go-apply/internal/presenter/mcp"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/sessionstore"
)

// sessions is the process-lifetime session store used by all MCP multi-turn handlers.
// CLI subcommands pass their own DiskStore via the store parameter on WithConfig variants.
var sessions sessionstore.Store = sessionstore.NewMemoryStore()

// HandleLoadJD is the exported handler for the "load_jd" MCP tool.
// Uses the process-level memory store.
func HandleLoadJD(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleLoadJDWithConfig(ctx, req, nil, nil)
}

// HandleLoadJDWithConfig is the full handler with optional injected deps and store.
// When deps is nil, loadDeps() is called. When store is nil, the process-level sessions store is used.
func HandleLoadJDWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig, store sessionstore.Store) *mcp.CallToolResult {
	if store == nil {
		store = sessions
	}

	jdURL := req.GetString("jd_url", "")
	jdRawText := req.GetString("jd_raw_text", "")
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "load_jd"),
		logger.PayloadAttr("jd_url", jdURL, logger.Verbose()),
		logger.PayloadAttr("jd_raw_text", jdRawText, logger.Verbose()),
	)
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

	sess, err := store.Create(ctx, jdText)
	if err != nil {
		slog.ErrorContext(ctx, "load_jd: create session failed", "error", err)
		return envelopeResult(stageErrorEnvelope("", "load_jd", "session_error", err.Error(), true))
	}
	sess.URL = jdURL
	sess.IsText = isText
	if err := store.Update(ctx, sess); err != nil {
		slog.WarnContext(ctx, "load_jd: persist session metadata failed", "session_id", sess.ID, "error", err)
	}
	logger.Banner(ctx, slog.Default(), "Session", sess.ID)

	type loadJDData struct {
		JDText string `json:"jd_text"`
	}
	resultData := loadJDData{JDText: jdText}
	resultBytes, _ := json.Marshal(resultData)
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "load_jd"),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(resultBytes)),
		logger.PayloadAttr("result", string(resultBytes), logger.Verbose()),
	)
	return envelopeResult(okEnvelope(sess.ID, "extract_keywords", resultData))
}

// HandleSubmitKeywords is the exported handler for the "submit_keywords" MCP tool.
func HandleSubmitKeywords(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleSubmitKeywordsWithConfig(ctx, req, nil, nil, nil)
}

// HandleSubmitKeywordsWithConfig is the full handler with optional injected deps and store.
func HandleSubmitKeywordsWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig, cfg *config.Config, store sessionstore.Store) *mcp.CallToolResult {
	if store == nil {
		store = sessions
	}

	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "submit_keywords", "missing_session", "session_id is required", false))
	}
	jdJSONStr := req.GetString("jd_json", "")
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "submit_keywords"),
		slog.String("session_id", sessionID),
		logger.PayloadAttr("jd_json", jdJSONStr, logger.Verbose()),
	)
	if jdJSONStr == "" {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "missing_jd", "jd_json is required", false))
	}

	sess, ok, err := store.Get(ctx, sessionID)
	if err != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "session_error", err.Error(), true))
	}
	if !ok {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State != sessionstore.StateLoaded {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "invalid_state",
			fmt.Sprintf("expected state %q, got %q — call load_jd first", sessionstore.StateLoaded, sess.State), false))
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

	logger.Banner(ctx, slog.Default(), "Score", "Initial")
	scored, err := pl.ScoreResumes(ctx, &jd, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "submit_keywords: score failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "score", "score_failed", err.Error(), false))
	}

	sess.JD = jd
	sess.ScoreResult = scored
	sess.State = sessionstore.StateScored
	if updateErr := store.Update(ctx, sess); updateErr != nil {
		slog.WarnContext(ctx, "submit_keywords: persist session failed", "session_id", sessionID, "error", updateErr)
	}

	nextAction := NextActionFromScore(scored.BestScore)

	type extractedKeywordsData struct {
		Title         string               `json:"title"`
		Company       string               `json:"company"`
		Required      []string             `json:"required"`
		Preferred     []string             `json:"preferred,omitempty"`
		Location      string               `json:"location,omitempty"`
		Seniority     model.SeniorityLevel `json:"seniority,omitempty"`
		RequiredYears float64              `json:"required_years,omitempty"`
	}
	type submitKeywordsData struct {
		ExtractedKeywords extractedKeywordsData        `json:"extracted_keywords"`
		Scores            map[string]model.ScoreResult `json:"scores"`
		BestResume        string                       `json:"best_resume"`
		BestScore         float64                      `json:"best_score"`
	}
	resultData := submitKeywordsData{
		ExtractedKeywords: extractedKeywordsData{
			Title:         jd.Title,
			Company:       jd.Company,
			Required:      jd.Required,
			Preferred:     jd.Preferred,
			Location:      jd.Location,
			Seniority:     jd.Seniority,
			RequiredYears: jd.RequiredYears,
		},
		Scores:     scored.Scores,
		BestResume: scored.BestLabel,
		BestScore:  scored.BestScore,
	}
	resultBytes, _ := json.Marshal(resultData)
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "submit_keywords"),
		slog.String("session_id", sessionID),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(resultBytes)),
		logger.PayloadAttr("result", string(resultBytes), logger.Verbose()),
	)
	return envelopeResult(okEnvelope(sessionID, nextAction, resultData))
}

// HandleFinalize is the exported handler for the "finalize" MCP tool.
func HandleFinalize(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleFinalizeWithConfig(ctx, req, nil, nil)
}

// HandleFinalizeWithConfig is the full handler with optional injected deps and store.
func HandleFinalizeWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig, store sessionstore.Store) *mcp.CallToolResult {
	if store == nil {
		store = sessions
	}

	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "finalize", "missing_session", "session_id is required", false))
	}
	coverLetter := req.GetString("cover_letter", "")
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "finalize"),
		slog.String("session_id", sessionID),
		logger.PayloadAttr("cover_letter", coverLetter, logger.Verbose()),
	)

	sess, ok, err := store.Get(ctx, sessionID)
	if err != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "finalize", "session_error", err.Error(), true))
	}
	if !ok {
		return envelopeResult(stageErrorEnvelope(sessionID, "finalize", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State == sessionstore.StateLoaded {
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
			if sess.TailoredText != "" || len(sess.Changelog) > 0 {
				rec.TailorResult = &model.TailorResult{
					ResumeLabel: sess.ScoreResult.BestLabel,
					NewScore:    sess.ScoreResult.Scores[sess.ScoreResult.BestLabel],
					Changelog:   sess.Changelog,
				}
			}
			if putErr := deps.AppRepo.Put(rec); putErr != nil {
				slog.WarnContext(ctx, "finalize: failed to persist record", "session_id", sessionID, "error", putErr)
			}
		}
	}

	sess.State = sessionstore.StateFinalized
	if updateErr := store.Update(ctx, sess); updateErr != nil {
		slog.WarnContext(ctx, "finalize: persist session state failed", "session_id", sessionID, "error", updateErr)
	}

	type finalizeSummary struct {
		KeywordsRequired  int     `json:"keywords_required"`
		KeywordsPreferred int     `json:"keywords_preferred"`
		ResumesScored     int     `json:"resumes_scored"`
		BestResume        string  `json:"best_resume"`
		BestScore         float64 `json:"best_score"`
		CoverLetterChars  int     `json:"cover_letter_chars"`
	}
	type finalizeData struct {
		BestResume  string          `json:"best_resume"`
		BestScore   float64         `json:"best_score"`
		CoverLetter string          `json:"cover_letter,omitempty"`
		Summary     finalizeSummary `json:"summary"`
	}
	resultData := finalizeData{
		BestResume:  sess.ScoreResult.BestLabel,
		BestScore:   sess.ScoreResult.BestScore,
		CoverLetter: coverLetter,
		Summary: finalizeSummary{
			KeywordsRequired:  len(sess.JD.Required),
			KeywordsPreferred: len(sess.JD.Preferred),
			ResumesScored:     len(sess.ScoreResult.Scores),
			BestResume:        sess.ScoreResult.BestLabel,
			BestScore:         sess.ScoreResult.BestScore,
			CoverLetterChars:  len(coverLetter),
		},
	}
	resultBytes, _ := json.Marshal(resultData)
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "finalize"),
		slog.String("session_id", sessionID),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(resultBytes)),
		logger.PayloadAttr("result", string(resultBytes), logger.Verbose()),
	)
	return envelopeResult(okEnvelope(sessionID, "", resultData))
}

// HandleSubmitTailoredResume is the exported handler for the "submit_tailored_resume" MCP tool.
func HandleSubmitTailoredResume(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleSubmitTailoredResumeWithConfig(ctx, req, nil, nil, nil)
}

// HandleSubmitTailoredResumeWithConfig is the full handler with optional injected deps and store.
func HandleSubmitTailoredResumeWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig, cfg *config.Config, store sessionstore.Store) *mcp.CallToolResult {
	if store == nil {
		store = sessions
	}

	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "submit_tailored_resume", "missing_session", "session_id is required", false))
	}
	tailoredText := req.GetString("tailored_text", "")
	changelogStr := req.GetString("changelog", "")

	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "submit_tailored_resume"),
		slog.String("session_id", sessionID),
		logger.PayloadAttr("tailored_text", tailoredText, logger.Verbose()),
	)

	// Validate tailored_text.
	if strings.TrimSpace(tailoredText) == "" {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "invalid_tailored_text",
			"tailored_text is required and must not be empty or whitespace-only", false))
	}

	// Parse and validate optional changelog.
	var entries []model.ChangelogEntry
	if changelogStr != "" {
		if err := json.Unmarshal([]byte(changelogStr), &entries); err != nil {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "invalid_changelog",
				fmt.Sprintf("changelog parse failed: %v", err), false))
		}
		for i, e := range entries {
			if !model.ValidateChangelogAction(e.Action) {
				return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "invalid_changelog",
					fmt.Sprintf("invalid action %q in changelog entry %d (allowed: added, rewrote, skipped)", e.Action, i), false))
			}
			if !model.ValidateChangelogTarget(e.Target) {
				return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "invalid_changelog",
					fmt.Sprintf("invalid target %q in changelog entry %d (allowed: skill, bullet, summary)", e.Target, i), false))
			}
			if len(e.Keyword) > 128 {
				return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "invalid_changelog",
					fmt.Sprintf("keyword in changelog entry %d exceeds 128 bytes (got %d)", i, len(e.Keyword)), false))
			}
			if len(e.Reason) > 512 {
				return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "invalid_changelog",
					fmt.Sprintf("reason in changelog entry %d exceeds 512 bytes (got %d)", i, len(e.Reason)), false))
			}
		}
	}

	// Look up session.
	sess, ok, err := store.Get(ctx, sessionID)
	if err != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "session_error", err.Error(), true))
	}
	if !ok {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "session_not_found",
			"session not found — call load_jd first", false))
	}

	// State guard: accept StateScored or StateTailored (retry after rescore failure).
	if sess.State != sessionstore.StateScored && sess.State != sessionstore.StateTailored {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "invalid_state",
			fmt.Sprintf("expected state %q or %q, got %q", sessionstore.StateScored, sessionstore.StateTailored, sess.State), false))
	}

	// Capture previous score before mutating state.
	prevScore := sess.ScoreResult.BestScore
	resumeLabel := sess.ScoreResult.BestLabel

	// Advance session state before rescore (retry-friendly).
	sess.TailoredText = tailoredText
	sess.Changelog = entries
	sess.State = sessionstore.StateTailored

	// Wire dependencies.
	if deps == nil {
		liveCfg, liveDeps, err := loadDeps()
		if err != nil {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "config_error", err.Error(), true))
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

	// Rescore the tailored resume.
	newScore, err := pl.ScoreResume(ctx, tailoredText, resumeLabel, &sess.JD, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "rescore failed",
			slog.String("session_id", sessionID),
			slog.String("resume_label", resumeLabel),
			slog.Any("error", err),
		)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailored_resume", "rescore_failed",
			"rescore failed; see server logs for details", true))
	}

	// On success, update scores.
	if sess.ScoreResult.Scores == nil {
		sess.ScoreResult.Scores = make(map[string]model.ScoreResult)
	}
	sess.ScoreResult.Scores[resumeLabel] = newScore
	sess.ScoreResult.BestScore = newScore.Breakdown.Total()

	newTotal := newScore.Breakdown.Total()

	if updateErr := store.Update(ctx, sess); updateErr != nil {
		slog.WarnContext(ctx, "submit_tailored_resume: persist session failed", "session_id", sessionID, "error", updateErr)
	}

	slog.InfoContext(ctx, "tailor submission complete",
		slog.Int("tailored_text_bytes", len(tailoredText)),
		slog.Int("tailored_text_lines", lineCount(tailoredText)),
		slog.Float64("previous_score", prevScore),
		slog.Float64("new_score", newTotal),
	)

	type submitTailoredResumeData struct {
		PreviousScore float64                `json:"previous_score"`
		NewScore      model.ScoreResult      `json:"new_score"`
		TailoredText  string                 `json:"tailored_text"`
		Changelog     []model.ChangelogEntry `json:"changelog,omitempty"`
	}
	resultData := submitTailoredResumeData{
		PreviousScore: prevScore,
		NewScore:      newScore,
		TailoredText:  tailoredText,
		Changelog:     entries,
	}
	resultBytes, _ := json.Marshal(resultData)
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "submit_tailored_resume"),
		slog.String("session_id", sessionID),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(resultBytes)),
		logger.PayloadAttr("result", string(resultBytes), logger.Verbose()),
	)
	return envelopeResult(okEnvelope(sessionID, "finalize", resultData))
}

// NextActionFromScore returns the recommended next action based on the best resume score.
// Score is on a 0–100 scale (sum of weighted breakdown components). Exported for testing.
func NextActionFromScore(score float64) string {
	switch {
	case score < 40.0:
		return "advise_skip"
	case score < 70.0:
		return "tailor_t1"
	default:
		return "cover_letter"
	}
}
