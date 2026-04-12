package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// TailorConfig holds all dependencies for a TailorPipeline.
// All service fields are interfaces — wire concrete adapters at the CLI layer only.
type TailorConfig struct {
	Fetcher   port.JDFetcher
	LLM       port.LLMClient
	Scorer    port.Scorer
	Tailor    port.Tailor
	Resumes   port.ResumeRepository
	JDCache   port.JDCacheRepository
	DocLoader port.DocumentLoader
	Augmenter port.Augmenter // may be nil — pipeline degrades gracefully
	Presenter port.Presenter
	Defaults  *config.AppDefaults
}

// TailorPipeline executes the tailor pipeline: fetch JD, extract keywords,
// find resume, optionally augment, score before, tailor, re-score, present.
type TailorPipeline struct {
	fetcher   port.JDFetcher
	llm       port.LLMClient
	scorer    port.Scorer
	tailor    port.Tailor
	resumes   port.ResumeRepository
	jdCache   port.JDCacheRepository
	loader    port.DocumentLoader
	augmenter port.Augmenter // may be nil
	presenter port.Presenter
	defaults  *config.AppDefaults
}

// NewTailorPipeline constructs a TailorPipeline from a TailorConfig.
func NewTailorPipeline(cfg TailorConfig) *TailorPipeline {
	return &TailorPipeline{
		fetcher:   cfg.Fetcher,
		llm:       cfg.LLM,
		scorer:    cfg.Scorer,
		tailor:    cfg.Tailor,
		resumes:   cfg.Resumes,
		jdCache:   cfg.JDCache,
		loader:    cfg.DocLoader,
		augmenter: cfg.Augmenter,
		presenter: cfg.Presenter,
		defaults:  cfg.Defaults,
	}
}

// TailorRequest is the per-invocation input to TailorPipeline.Run.
type TailorRequest struct {
	ResumeLabel string
	URL         string
	Text        string
	Cfg         *config.Config
}

// Run executes the full tailor pipeline for the given request.
func (p *TailorPipeline) Run(ctx context.Context, req TailorRequest) error {
	// ── Step 1: fetch JD ──────────────────────────────────────────────────────
	jdText, err := p.tailorStepFetch(ctx, req)
	if err != nil {
		p.presenter.ShowError(err)
		return err
	}

	// ── Step 2: extract keywords ──────────────────────────────────────────────
	jd := p.tailorStepKeywords(ctx, jdText)

	// ── Step 3: find resume ───────────────────────────────────────────────────
	resume, resumeText, err := p.tailorStepResume(req.ResumeLabel)
	if err != nil {
		p.presenter.ShowError(err)
		return err
	}

	// ── Step 4: augment (optional) ────────────────────────────────────────────
	augText := p.tailorStepAugment(ctx, resumeText, jd)

	// ── Step 5: score before ──────────────────────────────────────────────────
	scoreBefore, err := p.tailorStepScore("score_before", "Scoring resume", resume, augText, jd, req.Cfg)
	if err != nil {
		p.presenter.ShowError(err)
		return err
	}

	// ── Step 6: tailor ────────────────────────────────────────────────────────
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "tailor", Label: "Tailoring resume"})

	tailorInput := port.TailorInput{
		Resume:      resume,
		ResumeText:  augText,
		JD:          jd,
		ScoreBefore: scoreBefore,
		Options: port.TailorOptions{
			MaxTier2BulletRewrites: p.defaults.Tailor.MaxTier2BulletRewrites,
		},
	}
	tailorResult, err := p.tailor.TailorResume(ctx, tailorInput)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "tailor", Label: "Tailoring resume", Err: err.Error()})
		p.presenter.ShowError(fmt.Errorf("tailor resume: %w", err))
		return fmt.Errorf("tailor resume: %w", err)
	}
	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "tailor",
		Label:     "Tailoring resume",
		ElapsedMS: time.Since(start).Milliseconds(),
	})

	// ── Step 7: re-score tailored text ────────────────────────────────────────
	// Build the tailored text by applying keyword additions to augText for re-scoring.
	// We re-score using the same augmented text as baseline for the new score.
	newScore, scoreErr := p.tailorStepScore("score_after", "Re-scoring tailored resume", resume, augText, jd, req.Cfg)
	if scoreErr == nil {
		tailorResult.NewScore = newScore
	}

	// ── Present ───────────────────────────────────────────────────────────────
	return p.presenter.ShowTailorResult(&tailorResult)
}

// ── step helpers ──────────────────────────────────────────────────────────────

func (p *TailorPipeline) tailorStepFetch(ctx context.Context, req TailorRequest) (string, error) {
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "fetch", Label: "Fetching job description"})

	var jdText string
	switch {
	case req.Text != "":
		jdText = req.Text
	case req.URL != "":
		rawText, _, found := p.jdCache.Get(req.URL)
		if found {
			jdText = rawText
		} else {
			var err error
			jdText, err = p.fetcher.Fetch(ctx, req.URL)
			if err != nil {
				p.presenter.OnEvent(model.StepFailedEvent{StepID: "fetch", Label: "Fetching job description", Err: err.Error()})
				return "", fmt.Errorf("fetch JD from %s: %w", req.URL, err)
			}
		}
	default:
		err := fmt.Errorf("either URL or Text must be provided in TailorRequest")
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "fetch", Label: "Fetching job description", Err: err.Error()})
		return "", err
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "fetch",
		Label:     "Fetching job description",
		ElapsedMS: time.Since(start).Milliseconds(),
	})
	return jdText, nil
}

func (p *TailorPipeline) tailorStepKeywords(ctx context.Context, jdText string) model.JDData {
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "keywords", Label: "Extracting keywords"})

	jd, err := extractKeywords(ctx, p.llm, jdText, p.defaults)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "keywords", Label: "Extracting keywords", Err: err.Error()})
		return model.JDData{}
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "keywords",
		Label:     "Extracting keywords",
		ElapsedMS: time.Since(start).Milliseconds(),
	})
	return jd
}

func (p *TailorPipeline) tailorStepResume(label string) (model.ResumeFile, string, error) {
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "resume", Label: "Loading resume"})

	files, err := p.resumes.ListResumes()
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "resume", Label: "Loading resume", Err: err.Error()})
		return model.ResumeFile{}, "", fmt.Errorf("list resumes: %w", err)
	}

	var found *model.ResumeFile
	for i := range files {
		if files[i].Label == label {
			found = &files[i]
			break
		}
	}
	if found == nil {
		err := fmt.Errorf("resume %q not found", label)
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "resume", Label: "Loading resume", Err: err.Error()})
		return model.ResumeFile{}, "", err
	}

	resumeText, err := p.loader.Load(found.Path)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "resume", Label: "Loading resume", Err: err.Error()})
		return model.ResumeFile{}, "", fmt.Errorf("load resume %s: %w", found.Path, err)
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "resume",
		Label:     "Loading resume",
		ElapsedMS: time.Since(start).Milliseconds(),
	})
	return *found, resumeText, nil
}

func (p *TailorPipeline) tailorStepAugment(ctx context.Context, resumeText string, jd model.JDData) string {
	if p.augmenter == nil {
		return resumeText
	}

	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "augment", Label: "Augmenting resume"})

	keywords := make([]string, 0, len(jd.Required)+len(jd.Preferred))
	keywords = append(keywords, jd.Required...)
	keywords = append(keywords, jd.Preferred...)

	augmented, _, err := p.augmenter.AugmentResumeText(ctx, port.AugmentInput{
		ResumeText: resumeText,
		JDKeywords: keywords,
	})
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "augment", Label: "Augmenting resume", Err: err.Error()})
		return resumeText
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "augment",
		Label:     "Augmenting resume",
		ElapsedMS: time.Since(start).Milliseconds(),
	})
	return augmented
}

func (p *TailorPipeline) tailorStepScore(stepID, label string, resume model.ResumeFile, resumeText string, jd model.JDData, cfg *config.Config) (model.ScoreResult, error) {
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: stepID, Label: label})

	seniorityMatch := computeSeniorityMatch(cfg.DefaultSeniority, string(jd.Seniority))

	scoreResult, err := p.scorer.Score(port.ScorerInput{
		ResumeText:     resumeText,
		ResumeLabel:    resume.Label,
		ResumePath:     resume.Path,
		JD:             jd,
		CandidateYears: cfg.YearsOfExperience,
		RequiredYears:  jd.RequiredYears,
		SeniorityMatch: seniorityMatch,
	})
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: stepID, Label: label, Err: err.Error()})
		return model.ScoreResult{}, fmt.Errorf("score resume: %w", err)
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    stepID,
		Label:     label,
		ElapsedMS: time.Since(start).Milliseconds(),
	})
	return scoreResult, nil
}
