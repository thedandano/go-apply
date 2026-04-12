// Package pipeline wires all services together into a single Apply pipeline run.
// The pipeline is constructed once at CLI layer and executed per invocation.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// seniorityOrder maps seniority level strings to a numeric rank for comparison.
var seniorityOrder = map[string]int{
	"junior":   0,
	"mid":      1,
	"senior":   2,
	"lead":     3,
	"director": 4,
}

// ApplyPipeline executes the full apply pipeline: fetch JD, extract keywords,
// score resumes, optionally augment, generate cover letter, emit result.
type ApplyPipeline struct {
	fetcher   port.JDFetcher
	llm       port.LLMClient
	scorer    port.Scorer
	clGen     port.CoverLetterGenerator
	resumes   port.ResumeRepository
	jdCache   port.JDCacheRepository
	augmenter port.Augmenter // may be nil — degrades gracefully
	loader    port.DocumentLoader
	presenter port.Presenter
	defaults  *config.AppDefaults
	cfg       *config.Config
}

// Config holds the dependencies and configuration for an ApplyPipeline.
// All service fields are interfaces; wire concrete adapters at the CLI layer only.
type Config struct {
	Fetcher   port.JDFetcher
	LLM       port.LLMClient
	Scorer    port.Scorer
	CLGen     port.CoverLetterGenerator
	Resumes   port.ResumeRepository
	JDCache   port.JDCacheRepository
	Augmenter port.Augmenter // may be nil — pipeline degrades gracefully
	DocLoader port.DocumentLoader
	Presenter port.Presenter
	Defaults  *config.AppDefaults
	Cfg       *config.Config
}

// New constructs an ApplyPipeline. All fields are interfaces; no concrete adapters
// are constructed here. Wire at internal/cli layer only.
//
// cfg.Augmenter may be nil; the pipeline degrades gracefully when it is.
func New(cfg Config) *ApplyPipeline {
	return &ApplyPipeline{
		fetcher:   cfg.Fetcher,
		llm:       cfg.LLM,
		scorer:    cfg.Scorer,
		clGen:     cfg.CLGen,
		resumes:   cfg.Resumes,
		jdCache:   cfg.JDCache,
		augmenter: cfg.Augmenter,
		loader:    cfg.DocLoader,
		presenter: cfg.Presenter,
		defaults:  cfg.Defaults,
		cfg:       cfg.Cfg,
	}
}

// RunInput is the per-invocation input to ApplyPipeline.Run.
// URL and Text are mutually exclusive — set exactly one.
type RunInput struct {
	// URL is the job description URL. If set, the pipeline fetches it (with cache).
	URL string
	// Text is the raw job description text. If set, used directly without fetching.
	Text string
	// Channel is the application channel (COLD/REFERRAL/RECRUITER).
	Channel model.ChannelType
}

// Run executes the full apply pipeline for the given input.
// It emits StepStartedEvent/StepCompletedEvent/StepFailedEvent for each step.
// Non-fatal failures degrade gracefully — warnings are added to the result.
// Fatal failures (e.g., no resumes) call presenter.ShowError and return an error.
func (p *ApplyPipeline) Run(ctx context.Context, input RunInput) error {
	result := model.NewPipelineResult()
	result.StartTime = time.Now()
	result.Status = "ok"

	// ── Step 1: fetch ──────────────────────────────────────────────────────────
	jdText, err := p.stepFetch(ctx, input, result)
	if err != nil {
		p.presenter.ShowError(err)
		return err
	}

	// ── Step 2: keywords ──────────────────────────────────────────────────────
	jd := p.stepKeywords(ctx, jdText, result)
	result.JD = jd
	result.Keywords.Required = jd.Required
	result.Keywords.Preferred = jd.Preferred

	// ── Step 3: cache JD ──────────────────────────────────────────────────────
	if input.URL != "" && result.Status != "degraded" {
		if err := p.jdCache.Put(input.URL, jdText, jd); err != nil {
			result.Warnings = append(result.Warnings, model.RiskWarning{
				Severity: "warn",
				Message:  fmt.Sprintf("JD cache write failed: %v", err),
			})
		}
	}

	// ── Step 4: resumes ───────────────────────────────────────────────────────
	resumeFiles, err := p.stepResumes(result)
	if err != nil {
		p.presenter.ShowError(err)
		return err
	}

	// ── Step 5: score ─────────────────────────────────────────────────────────
	bestResume, bestText := p.stepScore(jd, resumeFiles, result)

	// ── Step 6: augment ───────────────────────────────────────────────────────
	augmentedText := p.stepAugment(ctx, bestResume, bestText, jd, result)

	// ── Step 7: cover letter ──────────────────────────────────────────────────
	p.stepCoverLetter(ctx, jd, input.Channel, augmentedText, result)

	// ── Finalise ──────────────────────────────────────────────────────────────
	result.EndTime = time.Now()
	return p.presenter.ShowResult(result)
}

// ── step helpers ──────────────────────────────────────────────────────────────

func (p *ApplyPipeline) stepFetch(ctx context.Context, input RunInput, _ *model.PipelineResult) (string, error) {
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "fetch", Label: "Fetching job description"})

	var jdText string

	switch {
	case input.Text != "":
		jdText = input.Text
	case input.URL != "":
		// Check cache first
		rawText, _, found := p.jdCache.Get(input.URL)
		if found {
			jdText = rawText
		} else {
			var err error
			jdText, err = p.fetcher.Fetch(ctx, input.URL)
			if err != nil {
				p.presenter.OnEvent(model.StepFailedEvent{StepID: "fetch", Label: "Fetching job description", Err: err.Error()})
				return "", fmt.Errorf("fetch JD from %s: %w", input.URL, err)
			}
		}
	default:
		err := fmt.Errorf("either URL or Text must be provided in RunInput")
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

func (p *ApplyPipeline) stepKeywords(ctx context.Context, jdText string, result *model.PipelineResult) model.JDData {
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "keywords", Label: "Extracting keywords"})

	jd, err := extractKeywords(ctx, p.llm, jdText, p.defaults)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "keywords", Label: "Extracting keywords", Err: err.Error()})
		result.Warnings = append(result.Warnings, model.RiskWarning{
			Severity: "warn",
			Message:  fmt.Sprintf("keyword extraction failed: %v", err),
		})
		result.Status = "degraded"
		return model.JDData{}
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "keywords",
		Label:     "Extracting keywords",
		ElapsedMS: time.Since(start).Milliseconds(),
	})
	return jd
}

func (p *ApplyPipeline) stepResumes(_ *model.PipelineResult) ([]model.ResumeFile, error) {
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "resumes", Label: "Loading resumes"})

	files, err := p.resumes.ListResumes()
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "resumes", Label: "Loading resumes", Err: err.Error()})
		return nil, fmt.Errorf("list resumes: %w", err)
	}
	if len(files) == 0 {
		err := fmt.Errorf("no resumes found in resume directory")
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "resumes", Label: "Loading resumes", Err: err.Error()})
		return nil, err
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "resumes",
		Label:     "Loading resumes",
		ElapsedMS: time.Since(start).Milliseconds(),
	})
	return files, nil
}

func (p *ApplyPipeline) stepScore(jd model.JDData, resumeFiles []model.ResumeFile, result *model.PipelineResult) (model.ResumeFile, string) {
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "score", Label: "Scoring resumes"})

	seniorityMatch := computeSeniorityMatch(p.cfg.DefaultSeniority, string(jd.Seniority))

	var bestFile model.ResumeFile
	var bestText string
	var bestTotal float64

	for _, rf := range resumeFiles {
		text, err := p.loader.Load(rf.Path)
		if err != nil {
			result.Warnings = append(result.Warnings, model.RiskWarning{
				Severity: "warn",
				Message:  fmt.Sprintf("could not load resume %s: %v", rf.Path, err),
			})
			continue
		}

		scoreResult, err := p.scorer.Score(port.ScorerInput{
			ResumeText:     text,
			ResumeLabel:    rf.Label,
			ResumePath:     rf.Path,
			JD:             jd,
			CandidateYears: p.cfg.YearsOfExperience,
			RequiredYears:  jd.RequiredYears,
			SeniorityMatch: seniorityMatch,
		})
		if err != nil {
			result.Warnings = append(result.Warnings, model.RiskWarning{
				Severity: "warn",
				Message:  fmt.Sprintf("scoring failed for %s: %v", rf.Label, err),
			})
			continue
		}

		result.Scores[rf.Label] = scoreResult
		total := scoreResult.Breakdown.Total()
		if total > bestTotal {
			bestTotal = total
			bestFile = rf
			bestText = text
		}
	}

	result.BestScore = bestTotal
	result.BestResume = bestFile.Label

	if len(result.Scores) == 0 {
		result.Warnings = append(result.Warnings, model.RiskWarning{
			Severity: "warn",
			Message:  "all resumes failed to load or score — no scores available",
		})
		result.Status = "degraded"
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "score",
		Label:     "Scoring resumes",
		ElapsedMS: time.Since(start).Milliseconds(),
	})
	return bestFile, bestText
}

func (p *ApplyPipeline) stepAugment(ctx context.Context, bestResume model.ResumeFile, bestText string, jd model.JDData, result *model.PipelineResult) string {
	if p.augmenter == nil {
		return bestText
	}

	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "augment", Label: "Augmenting resume"})

	keywords := make([]string, 0, len(jd.Required)+len(jd.Preferred))
	keywords = append(keywords, jd.Required...)
	keywords = append(keywords, jd.Preferred...)
	augmented, _, err := p.augmenter.AugmentResumeText(ctx, port.AugmentInput{
		ResumeText: bestText,
		JDKeywords: keywords,
	})
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "augment", Label: "Augmenting resume", Err: err.Error()})
		result.Warnings = append(result.Warnings, model.RiskWarning{
			Severity: "warn",
			Message:  fmt.Sprintf("augmentation failed for %s: %v", bestResume.Label, err),
		})
		result.Status = "degraded"
		return bestText
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "augment",
		Label:     "Augmenting resume",
		ElapsedMS: time.Since(start).Milliseconds(),
	})
	return augmented
}

// TODO: wire resumeText into port.CoverLetterInput once the interface is extended.
func (p *ApplyPipeline) stepCoverLetter(ctx context.Context, jd model.JDData, channel model.ChannelType, _ string, result *model.PipelineResult) {
	start := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "cover_letter", Label: "Generating cover letter"})

	profile := model.UserProfile{
		Name:              p.cfg.UserName,
		Occupation:        p.cfg.Occupation,
		Location:          p.cfg.Location,
		LinkedInURL:       p.cfg.LinkedInURL,
		YearsOfExperience: p.cfg.YearsOfExperience,
		Seniority:         p.cfg.DefaultSeniority,
	}

	clResult, err := p.clGen.Generate(ctx, &port.CoverLetterInput{
		JD:      jd,
		Scores:  result.Scores,
		Channel: channel,
		Profile: profile,
	})
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "cover_letter", Label: "Generating cover letter", Err: err.Error()})
		result.Warnings = append(result.Warnings, model.RiskWarning{
			Severity: "warn",
			Message:  fmt.Sprintf("cover letter generation failed: %v", err),
		})
		result.Status = "degraded"
		return
	}

	result.CoverLetter = clResult
	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "cover_letter",
		Label:     "Generating cover letter",
		ElapsedMS: time.Since(start).Milliseconds(),
	})
}

// extractKeywords calls the LLM to extract structured job description data from raw text.
func extractKeywords(ctx context.Context, llm port.LLMClient, jdText string, defaults *config.AppDefaults) (model.JDData, error) {
	systemPrompt := `You are a job description parser. Extract structured data from the job description text.
Return ONLY a JSON object with these fields:
{
  "title": "job title",
  "company": "company name",
  "required": ["required skill 1", "required skill 2"],
  "preferred": ["preferred skill 1", "preferred skill 2"],
  "location": "location or remote",
  "seniority": "junior|mid|senior|lead|director",
  "required_years": 0
}
Do not include any explanation, markdown, or extra text. Return only the JSON object.`

	messages := []port.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "Extract structured data from this job description:\n\n" + jdText},
	}

	opts := port.ChatOptions{
		Temperature: defaults.LLM.KeywordExtractionTemp,
		MaxTokens:   defaults.LLM.KeywordExtractionMaxTokens,
	}

	resp, err := llm.ChatComplete(ctx, messages, opts)
	if err != nil {
		return model.JDData{}, fmt.Errorf("keyword extraction LLM call: %w", err)
	}

	var jd model.JDData
	if err := json.Unmarshal([]byte(resp), &jd); err != nil {
		return model.JDData{}, fmt.Errorf("parse keyword extraction response: %w", err)
	}
	return jd, nil
}

// computeSeniorityMatch returns "exact", "one_off", or "two_or_more_off"
// based on how many levels apart candidateSeniority and jdSeniority are.
func computeSeniorityMatch(candidateSeniority, jdSeniority string) string {
	cRank, cOK := seniorityOrder[candidateSeniority]
	jRank, jOK := seniorityOrder[jdSeniority]
	if !cOK || !jOK {
		return "exact" // unknown level, treat as exact to avoid penalty
	}
	diff := int(math.Abs(float64(cRank - jRank)))
	switch {
	case diff == 0:
		return "exact"
	case diff == 1:
		return "one_off"
	default:
		return "two_or_more_off"
	}
}
