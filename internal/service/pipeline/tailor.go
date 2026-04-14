package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// TailorConfig holds all dependencies for a TailorPipeline.
type TailorConfig struct {
	Fetcher   port.JDFetcher
	LLM       port.LLMClient
	Scorer    port.Scorer
	Tailor    port.Tailor
	Resumes   port.ResumeRepository
	Loader    port.DocumentLoader
	AppRepo   port.ApplicationRepository
	Augment   port.Augmenter
	Presenter port.Presenter
	Defaults  *config.AppDefaults
}

// TailorRequest carries inputs for a single tailor pipeline execution.
type TailorRequest struct {
	// URLOrText is either a URL to fetch or raw JD text (when IsText is true).
	URLOrText string
	// IsText indicates that URLOrText is JD text, not a URL.
	IsText bool
	// ResumeLabel identifies which resume to tailor (case-insensitive match).
	ResumeLabel string
	// AccomplishmentsPath is an optional path to the accomplishments file for tier-2.
	AccomplishmentsPath string
	// Config is the active application configuration.
	Config *config.Config
}

// TailorPipeline orchestrates the tailor pipeline.
// All external dependencies are injected; no I/O is performed directly in the struct.
type TailorPipeline struct {
	fetcher   port.JDFetcher
	llm       port.LLMClient
	scorer    port.Scorer
	tailor    port.Tailor
	resumes   port.ResumeRepository
	loader    port.DocumentLoader
	appRepo   port.ApplicationRepository
	augment   port.Augmenter
	presenter port.Presenter
	defaults  *config.AppDefaults
}

// NewTailorPipeline constructs a TailorPipeline with all dependencies injected via TailorConfig.
func NewTailorPipeline(cfg *TailorConfig) *TailorPipeline {
	return &TailorPipeline{
		fetcher:   cfg.Fetcher,
		llm:       cfg.LLM,
		scorer:    cfg.Scorer,
		tailor:    cfg.Tailor,
		resumes:   cfg.Resumes,
		loader:    cfg.Loader,
		appRepo:   cfg.AppRepo,
		augment:   cfg.Augment,
		presenter: cfg.Presenter,
		defaults:  cfg.Defaults,
	}
}

// Run executes the full tailor pipeline and emits the result via the presenter.
// Returns a non-nil error only for unrecoverable infrastructure failures (fetch, resume load).
// Degraded states (augment, accomplishments load, score-after) are handled gracefully.
func (p *TailorPipeline) Run(ctx context.Context, req TailorRequest) error {
	// Step 1: fetch JD text.
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "fetch", Label: "Acquiring JD"})
	fetchStart := time.Now()
	jdText, err := p.acquireJDText(ctx, req)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "fetch", Label: "JD acquisition failed", Err: err.Error()})
		p.presenter.ShowError(err)
		return err
	}
	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "fetch",
		Label:     "JD acquired",
		ElapsedMS: time.Since(fetchStart).Milliseconds(),
	})

	// Step 2: extract keywords.
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "keywords", Label: "Extracting keywords"})
	kwStart := time.Now()
	jd, err := extractKeywordsFromText(ctx, p.llm, jdText, p.defaults)
	if err != nil {
		slog.WarnContext(ctx, "keyword extraction failed — continuing with empty JD", "error", err)
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "keywords", Label: "Keyword extraction failed", Err: err.Error()})
		jd = model.JDData{}
	} else {
		p.presenter.OnEvent(model.StepCompletedEvent{
			StepID:    "keywords",
			Label:     "Keywords extracted",
			ElapsedMS: time.Since(kwStart).Milliseconds(),
		})
	}

	// Step 3: find and load the resume.
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "resume", Label: "Loading resume"})
	resumeStart := time.Now()
	resumeFiles, err := p.resumes.ListResumes()
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "resume", Label: "List resumes failed", Err: err.Error()})
		p.presenter.ShowError(err)
		return fmt.Errorf("list resumes: %w", err)
	}

	resume, found := findResume(resumeFiles, req.ResumeLabel)
	if !found {
		err = fmt.Errorf("resume %q not found", req.ResumeLabel)
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "resume", Label: "Resume not found", Err: err.Error()})
		p.presenter.ShowError(err)
		return err
	}

	resumeText, err := p.loader.Load(resume.Path)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "resume", Label: "Resume load failed", Err: err.Error()})
		p.presenter.ShowError(err)
		return fmt.Errorf("load resume %q: %w", resume.Path, err)
	}
	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "resume",
		Label:     "Resume loaded",
		ElapsedMS: time.Since(resumeStart).Milliseconds(),
	})

	// Step 4: augment resume text.
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "augment", Label: "Augmenting resume"})
	augStart := time.Now()
	augmented, refData, augErr := p.augment.AugmentResumeText(ctx, model.AugmentInput{
		ResumeText: resumeText,
		RefData:    nil,
		JDKeywords: append(jd.Required, jd.Preferred...),
	})
	if augErr != nil {
		slog.WarnContext(ctx, "augmentation failed — using original text", "label", resume.Label, "error", augErr)
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "augment", Label: "Augment degraded", Err: augErr.Error()})
		augmented = resumeText
		refData = nil
	} else {
		p.presenter.OnEvent(model.StepCompletedEvent{
			StepID:    "augment",
			Label:     "Resume augmented",
			ElapsedMS: time.Since(augStart).Milliseconds(),
		})
	}

	// Step 5: score before tailoring.
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "score-before", Label: "Scoring resume (before)"})
	scoreBeforeStart := time.Now()
	seniorityMatch := resolveSeniorityMatch(req.Config.DefaultSeniority, &jd)
	scoreBefore, scoreErr := p.scorer.Score(&model.ScorerInput{
		ResumeText:     augmented,
		ResumeLabel:    resume.Label,
		ResumePath:     resume.Path,
		JD:             jd,
		CandidateYears: req.Config.YearsOfExperience,
		RequiredYears:  jd.RequiredYears,
		SeniorityMatch: seniorityMatch,
		ReferenceData:  refData,
	})
	if scoreErr != nil {
		slog.WarnContext(ctx, "pre-tailor scoring failed — continuing without baseline score", "error", scoreErr)
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "score-before", Label: "Score-before degraded", Err: scoreErr.Error()})
	} else {
		p.presenter.OnEvent(model.StepCompletedEvent{
			StepID:    "score-before",
			Label:     "Score-before complete",
			ElapsedMS: time.Since(scoreBeforeStart).Milliseconds(),
		})
	}

	// Step 6: load accomplishments file (optional).
	var accomplishmentsText string
	if req.AccomplishmentsPath != "" {
		p.presenter.OnEvent(model.StepStartedEvent{StepID: "accomplishments", Label: "Loading accomplishments"})
		accText, accErr := p.loader.Load(req.AccomplishmentsPath)
		if accErr != nil {
			slog.WarnContext(ctx, "accomplishments load failed — tier-2 will be skipped", "path", req.AccomplishmentsPath, "error", accErr)
			p.presenter.OnEvent(model.StepFailedEvent{StepID: "accomplishments", Label: "Accomplishments load degraded", Err: accErr.Error()})
		} else {
			accomplishmentsText = accText
			p.presenter.OnEvent(model.StepCompletedEvent{StepID: "accomplishments", Label: "Accomplishments loaded", ElapsedMS: 0})
		}
	}

	// Step 7: tailor the resume.
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "tailor", Label: "Tailoring resume"})
	tailorStart := time.Now()
	tailorInput := model.TailorInput{
		Resume:              resume,
		ResumeText:          augmented,
		JD:                  jd,
		ScoreBefore:         scoreBefore,
		AccomplishmentsText: accomplishmentsText,
		Options:             model.TailorOptions{MaxTier2BulletRewrites: p.defaults.Tailor.MaxTier2BulletRewrites},
	}
	tailorResult, err := p.tailor.TailorResume(ctx, &tailorInput)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "tailor", Label: "Tailor failed", Err: err.Error()})
		p.presenter.ShowError(err)
		return fmt.Errorf("tailor resume: %w", err)
	}
	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "tailor",
		Label:     "Resume tailored",
		ElapsedMS: time.Since(tailorStart).Milliseconds(),
	})

	// Step 8: score after tailoring using TailoredText for accurate delta.
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "score-after", Label: "Scoring resume (after)"})
	scoreAfterStart := time.Now()
	scoreAfterText := tailorResult.TailoredText
	if scoreAfterText == "" {
		scoreAfterText = augmented
	}
	scoreAfter, scoreAfterErr := p.scorer.Score(&model.ScorerInput{
		ResumeText:     scoreAfterText,
		ResumeLabel:    resume.Label,
		ResumePath:     resume.Path,
		JD:             jd,
		CandidateYears: req.Config.YearsOfExperience,
		RequiredYears:  jd.RequiredYears,
		SeniorityMatch: seniorityMatch,
		ReferenceData:  refData,
	})
	if scoreAfterErr != nil {
		slog.WarnContext(ctx, "post-tailor scoring failed — NewScore will be zero", "error", scoreAfterErr)
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "score-after", Label: "Score-after degraded", Err: scoreAfterErr.Error()})
	} else {
		p.presenter.OnEvent(model.StepCompletedEvent{
			StepID:    "score-after",
			Label:     "Score-after complete",
			ElapsedMS: time.Since(scoreAfterStart).Milliseconds(),
		})
		tailorResult.NewScore = scoreAfter
	}

	// Step 9 & 10: emit result.
	return p.presenter.ShowTailorResult(&tailorResult)
}

// acquireJDText returns the raw JD text, fetching from URL if needed.
// For text mode, it returns the input directly.
// For URL mode, it checks the cache first, then fetches.
func (p *TailorPipeline) acquireJDText(ctx context.Context, req TailorRequest) (string, error) {
	if req.IsText {
		return req.URLOrText, nil
	}

	// Check application cache first.
	rec, found, err := p.appRepo.Get(req.URLOrText)
	if err != nil {
		slog.WarnContext(ctx, "JD cache lookup error — proceeding with fetch", "url", req.URLOrText, "error", err)
	}
	if found && rec != nil && rec.RawText != "" {
		return rec.RawText, nil
	}

	// Fetch from URL.
	text, err := p.fetcher.Fetch(ctx, req.URLOrText)
	if err != nil {
		return "", fmt.Errorf("fetch JD from %s: %w", req.URLOrText, err)
	}
	return text, nil
}

// findResume finds a resume by label (case-insensitive) from a slice of resume files.
func findResume(resumes []model.ResumeFile, label string) (model.ResumeFile, bool) {
	for _, r := range resumes {
		if strings.EqualFold(r.Label, label) {
			return r, true
		}
	}
	return model.ResumeFile{}, false
}
