// Package pipeline orchestrates the MCP apply pipeline: fetch → score resumes → emit result.
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

// ApplyPipeline orchestrates the MCP apply pipeline.
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
	// PDFRenderer renders a SectionMap to PDF bytes for ATS extraction.
	// Required for preview_ats_extraction; must not be nil in production.
	PDFRenderer port.PDFRenderer
	// Extractor extracts plain text from PDF bytes for ATS extraction.
	// Required for preview_ats_extraction; must not be nil in production.
	Extractor port.Extractor
	// SurvivalDiffer computes which JD keywords survived the render→extract pipeline.
	// Required for preview_ats_extraction; must not be nil in production.
	SurvivalDiffer port.SurvivalDiffer
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
	if isText {
		return urlOrText, nil
	}

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

// resolveSeniorityMatch maps the candidate's configured seniority and the JD's
// detected seniority to one of the scorer's accepted keys: "exact", "one_off",
// "two_or_more_off".
func resolveSeniorityMatch(candidateSeniority string, jd *model.JDData) string {
	if candidateSeniority == "" {
		return "exact"
	}

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
