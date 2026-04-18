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
	"github.com/thedandano/go-apply/internal/port"
	mcppres "github.com/thedandano/go-apply/internal/presenter/mcp"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/tailor"
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

	sess := sessions.Create(jdText)
	sess.URL = jdURL
	sess.IsText = isText
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
	return HandleSubmitKeywordsWithConfig(ctx, req, nil, nil)
}

// HandleSubmitKeywordsWithConfig is the full handler with optional injected deps (for tests).
func HandleSubmitKeywordsWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig, cfg *config.Config) *mcp.CallToolResult {
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

	logger.Banner(ctx, slog.Default(), "Score", "Initial")
	scored, err := pl.ScoreResumes(ctx, &jd, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "submit_keywords: score failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "score", "score_failed", err.Error(), false))
	}

	sess.JD = jd
	sess.ScoreResult = scored
	sess.State = stateScored

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
	return HandleFinalizeWithConfig(ctx, req, nil)
}

// HandleFinalizeWithConfig is the full handler with optional injected deps (for tests).
func HandleFinalizeWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig) *mcp.CallToolResult {
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

// NextActionAfterT1 returns next_action after T1 tailoring — floored to tailor_t2 or cover_letter.
// Score is on a 0–100 scale.
func NextActionAfterT1(score float64) string {
	if score >= 70.0 {
		return "cover_letter"
	}
	return "tailor_t2"
}

// loadBestResumeText loads resume text for bestLabel from the repo.
func loadBestResumeText(deps *pipeline.ApplyConfig, bestLabel string) (string, error) {
	resumeFiles, err := deps.Resumes.ListResumes()
	if err != nil {
		return "", fmt.Errorf("list resumes: %w", err)
	}
	for _, r := range resumeFiles {
		if r.Label == bestLabel {
			text, loadErr := deps.Loader.Load(r.Path)
			if loadErr != nil {
				return "", fmt.Errorf("load resume %q: %w", bestLabel, loadErr)
			}
			return text, nil
		}
	}
	return "", fmt.Errorf("resume %q not found", bestLabel)
}

// HandleSubmitTailorT1 is the exported handler for the "submit_tailor_t1" MCP tool.
func HandleSubmitTailorT1(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleSubmitTailorT1WithConfig(ctx, req, nil, nil)
}

// HandleSubmitTailorT1WithConfig is the full handler with optional injected deps (for tests).
func HandleSubmitTailorT1WithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig, cfg *config.Config) *mcp.CallToolResult {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "submit_tailor_t1", "missing_session", "session_id is required", false))
	}
	skillAddsStr := req.GetString("skill_adds", "")
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "submit_tailor_t1"),
		slog.String("session_id", sessionID),
		logger.PayloadAttr("skill_adds", skillAddsStr, logger.Verbose()),
	)
	if skillAddsStr == "" {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "missing_skill_adds", "skill_adds is required", false))
	}
	var skillAdds []string
	if err := json.Unmarshal([]byte(skillAddsStr), &skillAdds); err != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "invalid_skill_adds",
			fmt.Sprintf("skill_adds parse failed: %v", err), false))
	}
	if len(skillAdds) == 0 {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "empty_skill_adds",
			"skill_adds must contain at least one skill", false))
	}

	sess := sessions.Get(sessionID)
	if sess == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State != stateScored {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "invalid_state",
			fmt.Sprintf("expected state %q, got %q", stateScored, sess.State), false))
	}

	if deps == nil {
		liveCfg, liveDeps, err := loadDeps()
		if err != nil {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "config_error", err.Error(), true))
		}
		deps = &liveDeps
		if cfg == nil {
			cfg = liveCfg
		}
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	baseText := sess.TailoredText
	if baseText == "" {
		text, err := loadBestResumeText(deps, sess.ScoreResult.BestLabel)
		if err != nil {
			slog.ErrorContext(ctx, "submit_tailor_t1: load resume failed", "session_id", sessionID, "error", err)
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "load_resume_failed", err.Error(), false))
		}
		baseText = text
	}

	pres := mcppres.New()
	deps.Presenter = pres
	pl := pipeline.NewApplyPipeline(deps)

	logger.Banner(ctx, slog.Default(), "Tailor", "T1")
	tailored, addedKeywords := tailor.AddKeywordsToSkillsSection(baseText, skillAdds)
	slog.InfoContext(ctx, "tailor T1 complete", "added_keywords", len(addedKeywords), "keywords", addedKeywords)

	logger.Banner(ctx, slog.Default(), "Score", "After T1")
	newScore, err := pl.ScoreResume(ctx, tailored, sess.ScoreResult.BestLabel, &sess.JD, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "submit_tailor_t1: rescore failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "rescore_failed", err.Error(), false))
	}
	// Commit to session only after successful rescore so a transient failure leaves the state retryable.
	sess.TailoredText = tailored
	sess.State = stateT1Applied

	previousScore := sess.ScoreResult.BestScore
	newScoreTotal := newScore.Breakdown.Total()
	if sess.ScoreResult.Scores == nil {
		sess.ScoreResult.Scores = make(map[string]model.ScoreResult)
	}
	sess.ScoreResult.Scores[sess.ScoreResult.BestLabel] = newScore
	sess.ScoreResult.BestScore = newScoreTotal

	type t1Data struct {
		PreviousScore float64           `json:"previous_score"`
		NewScore      model.ScoreResult `json:"new_score"`
		AddedKeywords []string          `json:"added_keywords"`
	}
	resultData := t1Data{
		PreviousScore: previousScore,
		NewScore:      newScore,
		AddedKeywords: addedKeywords,
	}
	resultBytes, _ := json.Marshal(resultData)
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "submit_tailor_t1"),
		slog.String("session_id", sessionID),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(resultBytes)),
		logger.PayloadAttr("result", string(resultBytes), logger.Verbose()),
	)
	return envelopeResult(okEnvelope(sessionID, NextActionAfterT1(newScoreTotal), resultData))
}

// HandleSubmitTailorT2 is the exported handler for the "submit_tailor_t2" MCP tool.
func HandleSubmitTailorT2(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandleSubmitTailorT2WithConfig(ctx, req, nil, nil)
}

// HandleSubmitTailorT2WithConfig is the full handler with optional injected deps (for tests).
func HandleSubmitTailorT2WithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig, cfg *config.Config) *mcp.CallToolResult {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "submit_tailor_t2", "missing_session", "session_id is required", false))
	}
	bulletRewritesStr := req.GetString("bullet_rewrites", "")
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "submit_tailor_t2"),
		slog.String("session_id", sessionID),
		logger.PayloadAttr("bullet_rewrites", bulletRewritesStr, logger.Verbose()),
	)
	if bulletRewritesStr == "" {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "missing_bullet_rewrites", "bullet_rewrites is required", false))
	}
	var rewrites []port.BulletRewrite
	if err := json.Unmarshal([]byte(bulletRewritesStr), &rewrites); err != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "invalid_bullet_rewrites",
			fmt.Sprintf("bullet_rewrites parse failed: %v", err), false))
	}
	if len(rewrites) == 0 {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "empty_bullet_rewrites",
			"bullet_rewrites must contain at least one entry", false))
	}

	sess := sessions.Get(sessionID)
	if sess == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State != stateT1Applied {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "invalid_state",
			fmt.Sprintf("expected state %q, got %q", stateT1Applied, sess.State), false))
	}

	if deps == nil {
		liveCfg, liveDeps, err := loadDeps()
		if err != nil {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "config_error", err.Error(), true))
		}
		deps = &liveDeps
		if cfg == nil {
			cfg = liveCfg
		}
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	baseText := sess.TailoredText
	if baseText == "" {
		text, err := loadBestResumeText(deps, sess.ScoreResult.BestLabel)
		if err != nil {
			slog.ErrorContext(ctx, "submit_tailor_t2: load resume failed", "session_id", sessionID, "error", err)
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "load_resume_failed", err.Error(), false))
		}
		baseText = text
	}

	pres := mcppres.New()
	deps.Presenter = pres
	pl := pipeline.NewApplyPipeline(deps)

	logger.Banner(ctx, slog.Default(), "Tailor", "T2")
	tailored, substitutionsMade := tailor.ApplyBulletRewrites(baseText, rewrites)
	slog.InfoContext(ctx, "tailor T2 complete", "substitutions_made", substitutionsMade)
	sess.TailoredText = tailored
	sess.State = stateT2Applied

	logger.Banner(ctx, slog.Default(), "Score", "After T2")
	newScore, err := pl.ScoreResume(ctx, tailored, sess.ScoreResult.BestLabel, &sess.JD, cfg)
	if err != nil {
		slog.ErrorContext(ctx, "submit_tailor_t2: rescore failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "rescore_failed", err.Error(), false))
	}

	previousScore := sess.ScoreResult.BestScore
	newScoreTotal := newScore.Breakdown.Total()
	if sess.ScoreResult.Scores == nil {
		sess.ScoreResult.Scores = make(map[string]model.ScoreResult)
	}
	sess.ScoreResult.Scores[sess.ScoreResult.BestLabel] = newScore
	sess.ScoreResult.BestScore = newScoreTotal

	type t2Data struct {
		PreviousScore     float64           `json:"previous_score"`
		NewScore          model.ScoreResult `json:"new_score"`
		SubstitutionsMade int               `json:"substitutions_made"`
	}
	resultData := t2Data{
		PreviousScore:     previousScore,
		NewScore:          newScore,
		SubstitutionsMade: substitutionsMade,
	}
	resultBytes, _ := json.Marshal(resultData)
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "submit_tailor_t2"),
		slog.String("session_id", sessionID),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(resultBytes)),
		logger.PayloadAttr("result", string(resultBytes), logger.Verbose()),
	)
	return envelopeResult(okEnvelope(sessionID, "cover_letter", resultData))
}
