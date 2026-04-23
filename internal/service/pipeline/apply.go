// Package pipeline orchestrates the scoring and JD-acquisition steps.
// The tailoring, cover-letter generation, and LLM keyword-extraction steps
// have been removed; the MCP host (Claude) now handles those concerns.
package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// ApplyPipeline orchestrates the scoring and JD-acquisition pipeline steps.
// All external dependencies are injected; no I/O is performed inside the struct directly.
type ApplyPipeline struct {
	fetcher   port.JDFetcher
	scorer    port.Scorer
	resumes   port.ResumeRepository
	loader    port.DocumentLoader
	appRepo   port.ApplicationRepository
	presenter port.Presenter
	defaults  *config.AppDefaults
}

// ApplyConfig holds all dependencies for an ApplyPipeline.
type ApplyConfig struct {
	Fetcher   port.JDFetcher
	Scorer    port.Scorer
	Resumes   port.ResumeRepository
	Loader    port.DocumentLoader
	AppRepo   port.ApplicationRepository
	Presenter port.Presenter
	Defaults  *config.AppDefaults
}

// NewApplyPipeline constructs an ApplyPipeline with all dependencies injected via ApplyConfig.
func NewApplyPipeline(cfg *ApplyConfig) *ApplyPipeline {
	return &ApplyPipeline{
		fetcher:   cfg.Fetcher,
		scorer:    cfg.Scorer,
		resumes:   cfg.Resumes,
		loader:    cfg.Loader,
		appRepo:   cfg.AppRepo,
		presenter: cfg.Presenter,
		defaults:  cfg.Defaults,
	}
}

// ScoreResumeResult holds the output of ScoreResumes.
type ScoreResumeResult struct {
	Scores    map[string]model.ScoreResult
	BestLabel string
	BestScore float64
}

// AcquireJD returns the raw JD text for the given URL or raw text input.
// It checks the local cache before fetching, emitting step events via the presenter.
func (p *ApplyPipeline) AcquireJD(ctx context.Context, urlOrText string, isText bool) (string, error) {
	return p.acquireJDText(ctx, urlOrText, isText)
}

// ScoreResumes lists all stored resumes and scores them against the given JD.
func (p *ApplyPipeline) ScoreResumes(ctx context.Context, jd *model.JDData, cfg *config.Config) (ScoreResumeResult, error) {
	resumeFiles, err := p.resumes.ListResumes()
	if err != nil {
		return ScoreResumeResult{}, fmt.Errorf("list resumes: %w", err)
	}
	// Warnings from skipped resumes are discarded here — this path is the MCP
	// score-only step where no PipelineResult accumulator exists.
	scores, bestLabel, bestScore, _, err := p.scoreResumes(ctx, resumeFiles, jd, cfg)
	if err != nil {
		return ScoreResumeResult{}, err
	}
	return ScoreResumeResult{
		Scores:    scores,
		BestLabel: bestLabel,
		BestScore: bestScore,
	}, nil
}

// ScoreResume scores a single resume text against a JD. It is a pure function:
// no I/O, no side effects — only the scorer is called.
func (p *ApplyPipeline) ScoreResume(_ context.Context, resumeText, resumeLabel string, jd *model.JDData, cfg *config.Config) (model.ScoreResult, error) {
	seniorityMatch := resolveSeniorityMatch(cfg.DefaultSeniority, jd)
	return p.scorer.Score(&model.ScorerInput{
		ResumeText:     resumeText,
		ResumeLabel:    resumeLabel,
		JD:             *jd,
		CandidateYears: cfg.YearsOfExperience,
		RequiredYears:  jd.RequiredYears,
		SeniorityMatch: seniorityMatch,
	})
}

// acquireJDText returns the raw JD text, either from the cache (for URLs) or
// by using the input directly (for text mode) or fetching (for URLs).
func (p *ApplyPipeline) acquireJDText(ctx context.Context, urlOrText string, isText bool) (string, error) {
	if isText {
		return urlOrText, nil
	}

	// Check cache first.
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "cache_lookup", Label: "Checking JD cache"})
	rec, found, err := p.appRepo.Get(urlOrText)
	if err != nil {
		slog.WarnContext(ctx, "cache lookup error — proceeding with fetch", "url", urlOrText, "error", err)
	}
	if found && rec != nil && rec.RawText != "" {
		slog.DebugContext(ctx, "jd: serving from cache", slog.String("url", urlOrText))
		p.presenter.OnEvent(model.StepCompletedEvent{StepID: "cache_lookup", Label: "Cache hit", ElapsedMS: 0})
		return rec.RawText, nil
	}
	slog.DebugContext(ctx, "jd: fetching from network — cache miss", slog.String("url", urlOrText))
	p.presenter.OnEvent(model.StepCompletedEvent{StepID: "cache_lookup", Label: "Cache miss — fetching", ElapsedMS: 0})

	// Fetch from URL.
	fetchStart := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "fetch", Label: "Fetching JD"})
	text, err := p.fetcher.Fetch(ctx, urlOrText)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "fetch", Label: "Fetch failed", Err: err.Error()})
		return "", fmt.Errorf("fetch JD from %s: %w", urlOrText, err)
	}
	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "fetch",
		Label:     "JD fetched",
		ElapsedMS: time.Since(fetchStart).Milliseconds(),
	})
	return text, nil
}

// scoreResumes scores each resume against the JD, returning the full scores map,
// the label of the best resume, and its score. skipped collects per-resume
// load/score failures as warnings for the caller to surface.
func (p *ApplyPipeline) scoreResumes(
	ctx context.Context,
	resumeFiles []model.ResumeFile,
	jd *model.JDData,
	cfg *config.Config,
) (map[string]model.ScoreResult, string, float64, []model.RiskWarning, error) {
	scoreStart := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "score", Label: "Scoring resumes"})

	scores := make(map[string]model.ScoreResult, len(resumeFiles))
	var bestLabel string
	var bestScore float64
	var skipped []model.RiskWarning

	for _, r := range resumeFiles {
		text, err := p.loader.Load(r.Path)
		if err != nil {
			slog.WarnContext(ctx, "failed to load resume — skipping", "path", r.Path, "error", err)
			skipped = append(skipped, model.RiskWarning{
				Severity: model.SeverityWarn,
				Message:  fmt.Sprintf("resume %q failed to load — skipped: %v", r.Label, err),
			})
			continue
		}

		sr, err := p.ScoreResume(ctx, text, r.Label, jd, cfg)
		if err != nil {
			slog.WarnContext(ctx, "scoring failed — skipping resume", "label", r.Label, "error", err)
			skipped = append(skipped, model.RiskWarning{
				Severity: model.SeverityWarn,
				Message:  fmt.Sprintf("resume %q failed to score — skipped: %v", r.Label, err),
			})
			continue
		}

		slog.DebugContext(ctx, "resume scored", "label", r.Label, "score", sr.Breakdown.Total())
		scores[r.Label] = sr
		total := sr.Breakdown.Total()
		if total > bestScore {
			bestScore = total
			bestLabel = r.Label
		}
	}

	if len(scores) == 0 && len(resumeFiles) > 0 {
		err := fmt.Errorf("scoring: all %d resume(s) failed to load or score", len(resumeFiles))
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "score", Label: "Score Resumes", Err: err.Error()})
		return nil, "", 0, skipped, err
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "score",
		Label:     "Scoring complete",
		ElapsedMS: time.Since(scoreStart).Milliseconds(),
	})
	return scores, bestLabel, bestScore, skipped, nil
}

// resolveSeniorityMatch maps the candidate's configured seniority and the JD's
// detected seniority to one of the scorer's accepted keys: "exact", "one_off",
// "two_or_more_off".
func resolveSeniorityMatch(candidateSeniority string, jd *model.JDData) string {
	// If the candidate has no configured seniority, default to exact.
	if candidateSeniority == "" {
		return "exact"
	}

	// Map seniority labels to numeric levels for distance calculation.
	levels := map[string]int{
		"junior":   1,
		"mid":      2,
		"senior":   3,
		"lead":     4,
		"director": 5,
	}

	candidateLevel, ok := levels[candidateSeniority]
	if !ok {
		return "exact"
	}

	jdLevel, ok := levels[string(jd.Seniority)]
	if !ok {
		return "exact"
	}

	diff := candidateLevel - jdLevel
	if diff < 0 {
		diff = -diff
	}

	switch diff {
	case 0:
		return "exact"
	case 1:
		return "one_off"
	default:
		return "two_or_more_off"
	}
}
