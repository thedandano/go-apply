package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/sync/errgroup"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	mcppres "github.com/thedandano/go-apply/internal/presenter/mcp"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	renderPkg "github.com/thedandano/go-apply/internal/service/render"
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
			slog.ErrorContext(ctx, "load_jd: dependency load failed", slog.Any("error", err))
			return envelopeResult(stageErrorEnvelope("", "load_jd", "config_error",
				"server configuration error — check server logs", true))
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
			slog.ErrorContext(ctx, "submit_keywords: dependency load failed",
				slog.String("session_id", sessionID), slog.Any("error", err))
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_keywords", "config_error",
				"server configuration error — check server logs", true))
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

	logger.Banner(ctx, slog.Default(), "Score", "Initial")

	// List all stored resumes.
	resumeFiles, listErr := deps.Resumes.ListResumes()
	if listErr != nil {
		slog.ErrorContext(ctx, "submit_keywords: list resumes failed", "session_id", sessionID, "error", listErr)
		return envelopeResult(stageErrorEnvelope(sessionID, "score", "score_failed", listErr.Error(), false))
	}

	// Fan-out: score each resume from its PDF representation via errgroup.
	// Hard error on any failure — no partial results.
	type resumeScoreEntry struct {
		label  string
		result model.ScoreResult
	}
	results := make([]resumeScoreEntry, len(resumeFiles))
	g, gctx := errgroup.WithContext(ctx)
	for i, rf := range resumeFiles {
		i, rf := i, rf
		g.Go(func() error {
			sections, secErr := deps.Resumes.LoadSections(rf.Label)
			if secErr != nil {
				return fmt.Errorf("submit_keywords: load sections for %s: %w", rf.Label, secErr)
			}
			sr, scoreErr := scoreSectionsPDF(gctx, &sections, rf.Label, sessionID, &jd, cfg, deps)
			if scoreErr != nil {
				return scoreErr
			}
			results[i] = resumeScoreEntry{label: rf.Label, result: sr}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		slog.ErrorContext(ctx, "submit_keywords: score failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "score", "score_failed", err.Error(), false))
	}

	// Build ScoreResumeResult from fan-out results.
	scores := make(map[string]model.ScoreResult, len(results))
	var bestLabel string
	var bestScore float64
	for i := range results {
		scores[results[i].label] = results[i].result
		if t := results[i].result.Breakdown.Total(); t > bestScore {
			bestScore = t
			bestLabel = results[i].label
		}
	}
	scored := pipeline.ScoreResumeResult{
		Scores:    scores,
		BestLabel: bestLabel,
		BestScore: bestScore,
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
		ExtractedKeywords  extractedKeywordsData        `json:"extracted_keywords"`
		Scores             map[string]model.ScoreResult `json:"scores"`
		BestResume         string                       `json:"best_resume"`
		BestScore          float64                      `json:"best_score"`
		SkillsSection      string                       `json:"skills_section"`
		SkillsSectionFound bool                         `json:"skills_section_found"`
		Sections           *model.SectionMap            `json:"sections,omitempty"`
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

	if sections, loadErr := deps.Resumes.LoadSections(scored.BestLabel); loadErr == nil {
		if sections.Skills != nil {
			resultData.SkillsSectionFound = true
			resultData.SkillsSection = sections.Skills.Flat
			if resultData.SkillsSection == "" && len(sections.Skills.Categorized) > 0 {
				var cats []string
				for cat := range sections.Skills.Categorized {
					cats = append(cats, cat)
				}
				sort.Strings(cats)
				for _, cat := range cats {
					resultData.SkillsSection += cat + ": " + strings.Join(sections.Skills.Categorized[cat], ", ") + "\n"
				}
				resultData.SkillsSection = strings.TrimRight(resultData.SkillsSection, "\n")
			}
		}
		resultData.Sections = &sections
		slog.InfoContext(ctx, "submit_keywords: skills_section processed",
			slog.String("session_id", sessionID),
			slog.Bool("skills_section_found", resultData.SkillsSectionFound),
		)
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
			if err != nil {
				slog.WarnContext(ctx, "finalize: dependency load failed; skipping persistence",
					slog.String("session_id", sessionID), slog.Any("error", err))
			} else {
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

	editsStr := req.GetString("edits", "")
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "submit_tailor_t1"),
		slog.String("session_id", sessionID),
		logger.PayloadAttr("edits", editsStr, logger.Verbose()),
	)
	if editsStr == "" {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "missing_edits", "edits is required", false))
	}

	var edits []port.Edit
	if err := json.Unmarshal([]byte(editsStr), &edits); err != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "invalid_edits",
			fmt.Sprintf("edits parse failed: %v", err), false))
	}
	if len(edits) == 0 {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "empty_edits",
			"edits must contain at least one entry", false))
	}
	for _, e := range edits {
		if e.Section != "skills" {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "invalid_section",
				fmt.Sprintf("submit_tailor_t1 only accepts section %q; got %q", "skills", e.Section), false))
		}
	}

	sess := sessions.Get(sessionID)
	if sess == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State != stateScored && sess.State != stateT1Applied {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "invalid_state",
			fmt.Sprintf("expected state %q or %q, got %q", stateScored, stateT1Applied, sess.State), false))
	}

	if deps == nil {
		liveCfg, liveDeps, err := loadDeps()
		if err != nil {
			slog.ErrorContext(ctx, "submit_tailor_t1: dependency load failed",
				slog.String("session_id", sessionID), slog.Any("error", err))
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "config_error",
				"server configuration error — check server logs", true))
		}
		deps = &liveDeps
		if cfg == nil {
			cfg = liveCfg
		}
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	if deps.Defaults != nil {
		maxEdits := deps.Defaults.Tailor.MaxTier1SkillRewrites
		if maxEdits > 0 && len(edits) > maxEdits {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "too_many_edits",
				fmt.Sprintf("edits length %d exceeds maximum %d", len(edits), maxEdits), false))
		}
	}

	// T1 always starts from the base sidecar — it replaces skills from scratch.
	// T2 chains from sess.TailoredSections (set by T1) so experience edits stack on top.
	sections, err := deps.Resumes.LoadSections(sess.ScoreResult.BestLabel)
	if err != nil {
		if errors.Is(err, model.ErrSectionsMissing) {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "sections_missing",
				"no sections found for this resume — re-onboard with sections field", false))
		}
		slog.ErrorContext(ctx, "submit_tailor_t1: load sections failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "load_sections_failed", err.Error(), false))
	}

	pres := mcppres.New()
	deps.Presenter = pres

	logger.Banner(ctx, slog.Default(), "Tailor", "T1")
	svc := tailor.New(nil, deps.Defaults, slog.Default())
	editResult, editErr := svc.ApplyEdits(ctx, sections, edits)
	if editErr != nil {
		slog.ErrorContext(ctx, "submit_tailor_t1: apply edits failed", "session_id", sessionID, "error", editErr)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "apply_edits_failed", editErr.Error(), false))
	}

	tailored, renderErr := renderSvc.Render(&editResult.NewSections)
	if renderErr != nil {
		slog.ErrorContext(ctx, "submit_tailor_t1: render failed", "session_id", sessionID, "error", renderErr)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "render_failed", renderErr.Error(), false))
	}

	slog.InfoContext(ctx, "tailor T1 complete",
		slog.String("session_id", sessionID),
		slog.Int("edits_applied", len(editResult.EditsApplied)),
		slog.Int("edits_rejected", len(editResult.EditsRejected)),
	)
	sess.TailoredText = tailored
	sess.TailoredSections = &editResult.NewSections
	sess.State = stateT1Applied

	logger.Banner(ctx, slog.Default(), "Score", "After T1")
	newScore, err := scoreSectionsPDF(ctx, &editResult.NewSections, sess.ScoreResult.BestLabel, sessionID, &sess.JD, cfg, deps)
	if err != nil {
		slog.ErrorContext(ctx, "submit_tailor_t1: rescore failed", "session_id", sessionID, "error", err)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t1", "rescore_failed", err.Error(), false))
	}

	previousScore := sess.ScoreResult.BestScore
	newScoreTotal := newScore.Breakdown.Total()
	if sess.ScoreResult.Scores == nil {
		sess.ScoreResult.Scores = make(map[string]model.ScoreResult)
	}
	sess.ScoreResult.Scores[sess.ScoreResult.BestLabel] = newScore
	sess.ScoreResult.BestScore = newScoreTotal

	type t1Data struct {
		PreviousScore float64              `json:"previous_score"`
		NewScore      model.ScoreResult    `json:"new_score"`
		EditsApplied  int                  `json:"edits_applied"`
		EditsRejected []port.EditRejection `json:"edits_rejected"`
		ScoringMethod string               `json:"scoring_method"`
	}
	resultData := t1Data{
		PreviousScore: previousScore,
		NewScore:      newScore,
		EditsApplied:  len(editResult.EditsApplied),
		EditsRejected: editResult.EditsRejected,
		ScoringMethod: ScoringMethodPDFExtracted,
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
	editsStr := req.GetString("edits", "")
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "submit_tailor_t2"),
		slog.String("session_id", sessionID),
		logger.PayloadAttr("edits", editsStr, logger.Verbose()),
	)
	if editsStr == "" {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "missing_edits", "edits is required", false))
	}
	var edits []port.Edit
	if err := json.Unmarshal([]byte(editsStr), &edits); err != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "invalid_edits",
			fmt.Sprintf("edits parse failed: %v", err), false))
	}
	if len(edits) == 0 {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "empty_edits",
			"edits must contain at least one entry", false))
	}
	for _, e := range edits {
		if e.Section != "experience" {
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "invalid_section",
				fmt.Sprintf("submit_tailor_t2 only accepts section %q; got %q", "experience", e.Section), false))
		}
	}

	sess := sessions.Get(sessionID)
	if sess == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State != stateScored && sess.State != stateT1Applied {
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "invalid_state",
			fmt.Sprintf("expected state %q or %q, got %q", stateScored, stateT1Applied, sess.State), false))
	}

	if deps == nil {
		liveCfg, liveDeps, err := loadDeps()
		if err != nil {
			slog.ErrorContext(ctx, "submit_tailor_t2: dependency load failed",
				slog.String("session_id", sessionID), slog.Any("error", err))
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "config_error",
				"server configuration error — check server logs", true))
		}
		deps = &liveDeps
		if cfg == nil {
			cfg = liveCfg
		}
	}
	if cfg == nil {
		cfg = &config.Config{}
	}

	var sections model.SectionMap
	if sess.TailoredSections != nil {
		sections = *sess.TailoredSections
	} else {
		loaded, loadErr := deps.Resumes.LoadSections(sess.ScoreResult.BestLabel)
		if loadErr != nil {
			if errors.Is(loadErr, model.ErrSectionsMissing) {
				return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "sections_missing",
					"no sections found for this resume — re-onboard with sections field", false))
			}
			slog.ErrorContext(ctx, "submit_tailor_t2: load sections failed", "session_id", sessionID, "error", loadErr)
			return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "load_sections_failed", loadErr.Error(), false))
		}
		sections = loaded
	}

	pres := mcppres.New()
	deps.Presenter = pres

	logger.Banner(ctx, slog.Default(), "Tailor", "T2")
	svc := tailor.New(nil, deps.Defaults, slog.Default())
	editResult, editErr := svc.ApplyEdits(ctx, sections, edits)
	if editErr != nil {
		slog.ErrorContext(ctx, "submit_tailor_t2: apply edits failed", "session_id", sessionID, "error", editErr)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "apply_edits_failed", editErr.Error(), false))
	}

	tailored, renderErr := renderSvc.Render(&editResult.NewSections)
	if renderErr != nil {
		slog.ErrorContext(ctx, "submit_tailor_t2: render failed", "session_id", sessionID, "error", renderErr)
		return envelopeResult(stageErrorEnvelope(sessionID, "submit_tailor_t2", "render_failed", renderErr.Error(), false))
	}

	slog.InfoContext(ctx, "tailor T2 complete",
		slog.String("session_id", sessionID),
		slog.Int("edits_applied", len(editResult.EditsApplied)),
		slog.Int("edits_rejected", len(editResult.EditsRejected)),
	)
	sess.TailoredText = tailored
	sess.State = stateT2Applied

	logger.Banner(ctx, slog.Default(), "Score", "After T2")
	newScore, err := scoreSectionsPDF(ctx, &editResult.NewSections, sess.ScoreResult.BestLabel, sessionID, &sess.JD, cfg, deps)
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
		PreviousScore float64              `json:"previous_score"`
		NewScore      model.ScoreResult    `json:"new_score"`
		EditsApplied  int                  `json:"edits_applied"`
		EditsRejected []port.EditRejection `json:"edits_rejected"`
		ScoringMethod string               `json:"scoring_method"`
	}
	resultData := t2Data{
		PreviousScore: previousScore,
		NewScore:      newScore,
		EditsApplied:  len(editResult.EditsApplied),
		EditsRejected: editResult.EditsRejected,
		ScoringMethod: ScoringMethodPDFExtracted,
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

// HandlePreviewATSExtraction is the exported handler for the "preview_ats_extraction" MCP tool.
func HandlePreviewATSExtraction(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	return HandlePreviewATSExtractionWithConfig(ctx, req, nil)
}

// renderSvc is the package-level text renderer used by the T1/T2 tailor handlers.
var renderSvc = renderPkg.New()

// HandlePreviewATSExtractionWithConfig is the full handler with optional injected deps (for tests).
// Renders the best resume to PDF, extracts plain text via the configured Extractor, and returns a
// keyword-survival diff showing which JD keywords survived the render→extract pipeline.
func HandlePreviewATSExtractionWithConfig(ctx context.Context, req *mcp.CallToolRequest, deps *pipeline.ApplyConfig) *mcp.CallToolResult {
	sessionID := req.GetString("session_id", "")
	if sessionID == "" {
		return envelopeResult(stageErrorEnvelope("", "preview_ats_extraction", "missing_session", "session_id is required", false))
	}
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "preview_ats_extraction"),
		slog.String("session_id", sessionID),
	)

	sess := sessions.Get(sessionID)
	if sess == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "preview_ats_extraction", "session_not_found",
			"session not found — call load_jd first", false))
	}
	if sess.State < stateScored {
		return envelopeResult(stageErrorEnvelope(sessionID, "preview_ats_extraction", "invalid_state",
			"session must be scored before previewing — call submit_keywords first", false))
	}

	if deps == nil {
		_, liveDeps, err := loadDeps()
		if err != nil {
			slog.ErrorContext(ctx, "preview_ats_extraction: dependency load failed",
				slog.String("session_id", sessionID), slog.Any("error", err))
			return envelopeResult(stageErrorEnvelope(sessionID, "preview_ats_extraction", "config_error",
				"server configuration error — check server logs", true))
		}
		deps = &liveDeps
	}

	label := sess.ScoreResult.BestLabel

	type previewData struct {
		Label           string `json:"label"`
		ConstructedText string `json:"constructed_text"`
		// SectionsUsed is always true in a success response (no fallback path exists after FR-005b).
		SectionsUsed    bool                  `json:"sections_used"`
		KeywordSurvival model.KeywordSurvival `json:"keyword_survival"`
	}
	pd := previewData{Label: label}

	// LoadSections first: user-fixable error (missing resume) takes priority over server misconfig.
	sections, sectErr := deps.Resumes.LoadSections(label)
	if sectErr != nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "preview_ats_extraction", "no_sections_data",
			"no structured resume sections available — upload a resume with sections sidecar", false))
	}

	if deps.PDFRenderer == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "preview_ats_extraction", "configuration_error",
			"PDFRenderer not configured — this is a server misconfiguration", false))
	}
	if deps.Extractor == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "preview_ats_extraction", "configuration_error",
			"Extractor not configured — this is a server misconfiguration", false))
	}
	if deps.SurvivalDiffer == nil {
		return envelopeResult(stageErrorEnvelope(sessionID, "preview_ats_extraction", "configuration_error",
			"SurvivalDiffer not configured — this is a server misconfiguration", false))
	}

	pdfBytes, renderErr := deps.PDFRenderer.RenderPDF(&sections)
	if renderErr != nil {
		slog.ErrorContext(ctx, "preview_ats_extraction: render failed",
			slog.String("session_id", sessionID), slog.Any("error", renderErr))
		return envelopeResult(stageErrorEnvelope(sessionID, "preview_ats_extraction", "render_failed",
			"PDF rendering failed — check server logs for details", false))
	}

	extracted, extErr := deps.Extractor.Extract(ctx, pdfBytes)
	if extErr != nil {
		slog.ErrorContext(ctx, "preview_ats_extraction: extract failed",
			slog.String("session_id", sessionID), slog.Any("error", extErr))
		return envelopeResult(stageErrorEnvelope(sessionID, "preview_ats_extraction", "extract_failed",
			"text extraction failed — check server logs for details", false))
	}

	pd.ConstructedText = extracted
	pd.SectionsUsed = true

	// Derive deduplicated keyword list from ScoreResult for the best resume.
	// Allocate a fresh slice to avoid aliasing the session's stored Keywords slices.
	kw := sess.ScoreResult.Scores[label].Keywords
	raw := make([]string, 0, len(kw.ReqMatched)+len(kw.ReqUnmatched)+len(kw.PrefMatched)+len(kw.PrefUnmatched))
	raw = append(raw, kw.ReqMatched...)
	raw = append(raw, kw.ReqUnmatched...)
	raw = append(raw, kw.PrefMatched...)
	raw = append(raw, kw.PrefUnmatched...)
	seen := make(map[string]struct{}, len(raw))
	unique := make([]string, 0, len(raw))
	for _, k := range raw {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			unique = append(unique, k)
		}
	}
	pd.KeywordSurvival = deps.SurvivalDiffer.Diff(unique, extracted)

	resultBytes, _ := json.Marshal(pd)
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "preview_ats_extraction"),
		slog.String("session_id", sessionID),
		slog.String("label", label),
		slog.Int("result_bytes", len(resultBytes)),
		logger.PayloadAttr("result", string(resultBytes), logger.Verbose()),
	)
	return envelopeResult(okEnvelope(sessionID, "", pd))
}
