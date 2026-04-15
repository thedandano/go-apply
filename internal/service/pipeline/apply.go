// Package pipeline orchestrates the full apply pipeline: fetch → extract keywords →
// score resumes → augment → tailor (optional) → generate cover letter → emit result.
package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// ApplyRequest carries inputs for a single pipeline execution.
type ApplyRequest struct {
	// URLOrText is either a URL to fetch or raw JD text (when IsText is true).
	URLOrText string
	// IsText indicates that URLOrText is JD text, not a URL.
	IsText bool
	// Channel is the application channel (COLD, REFERRAL, RECRUITER).
	Channel model.ChannelType
	// Config is the active application configuration.
	Config *config.Config
	// AccomplishmentsText is optional raw accomplishments text for tier-2
	// bullet rewriting. When empty, the tailor step is skipped.
	AccomplishmentsText string
}

// ApplyPipeline orchestrates the full headless apply pipeline.
// All external dependencies are injected; no I/O is performed inside the struct directly.
type ApplyPipeline struct {
	fetcher   port.JDFetcher
	llm       port.LLMClient
	scorer    port.Scorer
	clGen     port.CoverLetterGenerator
	resumes   port.ResumeRepository
	loader    port.DocumentLoader
	appRepo   port.ApplicationRepository
	augment   port.Augmenter
	presenter port.Presenter
	defaults  *config.AppDefaults
	tailor    port.Tailor
}

// ApplyConfig holds all dependencies for an ApplyPipeline.
type ApplyConfig struct {
	Fetcher   port.JDFetcher
	LLM       port.LLMClient
	Scorer    port.Scorer
	CLGen     port.CoverLetterGenerator
	Resumes   port.ResumeRepository
	Loader    port.DocumentLoader
	AppRepo   port.ApplicationRepository
	Augment   port.Augmenter
	Presenter port.Presenter
	Defaults  *config.AppDefaults
	Tailor    port.Tailor
}

// NewApplyPipeline constructs an ApplyPipeline with all dependencies injected via ApplyConfig.
func NewApplyPipeline(cfg *ApplyConfig) *ApplyPipeline {
	return &ApplyPipeline{
		fetcher:   cfg.Fetcher,
		llm:       cfg.LLM,
		scorer:    cfg.Scorer,
		clGen:     cfg.CLGen,
		resumes:   cfg.Resumes,
		loader:    cfg.Loader,
		appRepo:   cfg.AppRepo,
		augment:   cfg.Augment,
		presenter: cfg.Presenter,
		defaults:  cfg.Defaults,
		tailor:    cfg.Tailor,
	}
}

// Run executes the full apply pipeline and emits the result via the presenter.
// Returns a non-nil error only for unrecoverable infrastructure failures.
// Degraded pipeline states (e.g. failed keyword extraction) are captured in
// result.Status and result.Message rather than returned as errors.
func (p *ApplyPipeline) Run(ctx context.Context, req ApplyRequest) error {
	start := time.Now()
	result := model.NewPipelineResult()
	result.StartTime = start

	// Step 1: Acquire JD text.
	jdText, err := p.acquireJDText(ctx, req)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		result.EndTime = time.Now()
		p.presenter.ShowError(err)
		return err
	}
	result.JDText = jdText // returned so the MCP host (Claude) can extract keywords and reason over it

	// Step 2: Extract keywords from JD text via LLM.
	jd, degraded, kwErr := p.extractKeywords(ctx, jdText)
	if kwErr != nil {
		slog.WarnContext(ctx, "keyword extraction failed — continuing with empty JD", "error", kwErr)
		result.Status = "degraded"
		result.Message = fmt.Sprintf("keyword extraction failed: %v", kwErr)
		result.Error = kwErr.Error()
	}
	result.JD = jd
	result.Keywords.Required = jd.Required
	result.Keywords.Preferred = jd.Preferred

	// Cache the application record after keyword extraction.
	if !req.IsText {
		rec := &model.ApplicationRecord{
			URL:     req.URLOrText,
			RawText: jdText,
			JD:      jd,
		}
		if putErr := p.appRepo.Put(rec); putErr != nil {
			slog.WarnContext(ctx, "failed to cache application record", "url", req.URLOrText, "error", putErr)
		}
	}

	// Step 3: List and score resumes.
	resumeFiles, err := p.resumes.ListResumes()
	if err != nil {
		result.Status = "error"
		result.Error = fmt.Sprintf("list resumes: %v", err)
		result.EndTime = time.Now()
		p.presenter.ShowError(err)
		return fmt.Errorf("list resumes: %w", err)
	}

	scores, bestLabel, bestScore, err := p.scoreResumes(ctx, resumeFiles, &jd, req.Config)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		result.EndTime = time.Now()
		p.presenter.ShowError(err)
		return err
	}
	result.Scores = scores
	result.BestResume = bestLabel
	result.BestScore = bestScore

	// Step 4 (optional): Tailor the best-matching resume when --accomplishments is set.
	if req.AccomplishmentsText != "" && result.BestResume != "" && p.tailor != nil {
		tailorStart := time.Now()
		p.presenter.OnEvent(model.StepStartedEvent{StepID: "tailor", Label: "Tailoring resume"})
		tailorResult, tailorErr := p.runTailorStep(ctx, result, &jd, req, resumeFiles)
		if tailorErr != nil {
			slog.WarnContext(ctx, "tailor step failed — continuing", "error", tailorErr)
			p.presenter.OnEvent(model.StepFailedEvent{StepID: "tailor", Label: "Tailor failed", Err: tailorErr.Error()})
			result.Warnings = append(result.Warnings, model.RiskWarning{
				Severity: "warn",
				Message:  fmt.Sprintf("tailor step failed: %v", tailorErr),
			})
		} else {
			p.presenter.OnEvent(model.StepCompletedEvent{
				StepID:    "tailor",
				Label:     "Resume tailored",
				ElapsedMS: time.Since(tailorStart).Milliseconds(),
			})
			result.Cascade = tailorResult
			// Update best score if tailored version scores higher.
			if tailored := tailorResult.NewScore.Breakdown.Total(); tailored > result.BestScore {
				result.BestScore = tailored
			}
		}
	}

	// Step 5: Generate cover letter — only when CLGen is configured and score meets threshold.
	if p.clGen != nil && result.BestScore >= p.defaults.Thresholds.ScorePass {
		clStart := time.Now()
		p.presenter.OnEvent(model.StepStartedEvent{StepID: "05", Label: "Cover Letter"})
		clResult, clErr := p.clGen.Generate(ctx, &model.CoverLetterInput{
			JD:        jd,
			JDRawText: jdText,
			Scores:    scores,
			Channel:   req.Channel,
			Profile:   profileFromConfig(req.Config),
		})
		if clErr != nil {
			slog.WarnContext(ctx, "cover letter generation failed", "error", clErr)
			p.presenter.OnEvent(model.StepFailedEvent{StepID: "05", Label: "Cover Letter failed", Err: clErr.Error()})
			result.Warnings = append(result.Warnings, model.RiskWarning{
				Severity: "warn",
				Message:  fmt.Sprintf("cover letter generation failed: %v", clErr),
			})
		} else {
			p.presenter.OnEvent(model.StepCompletedEvent{
				StepID:    "05",
				Label:     "Cover Letter generated",
				ElapsedMS: time.Since(clStart).Milliseconds(),
			})
			result.CoverLetter = clResult
		}
	} else if p.clGen == nil {
		slog.InfoContext(ctx, "cover letter skipped — CLGen not configured (MCP mode: Claude generates cover letters)")
	} else {
		slog.InfoContext(ctx, "cover letter skipped — best score below threshold",
			"best_score", result.BestScore,
			"threshold", p.defaults.Thresholds.ScorePass,
		)
	}

	// Finalize result status.
	if result.Status == "" {
		if degraded {
			result.Status = "degraded"
		} else {
			result.Status = "success"
		}
	}
	result.EndTime = time.Now()

	return p.presenter.ShowResult(result)
}

// acquireJDText returns the raw JD text, either from the cache (for URLs) or
// by using the input directly (for text mode) or fetching (for URLs).
func (p *ApplyPipeline) acquireJDText(ctx context.Context, req ApplyRequest) (string, error) {
	if req.IsText {
		return req.URLOrText, nil
	}

	// Check cache first.
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "cache_lookup", Label: "Checking JD cache"})
	rec, found, err := p.appRepo.Get(req.URLOrText)
	if err != nil {
		slog.WarnContext(ctx, "cache lookup error — proceeding with fetch", "url", req.URLOrText, "error", err)
	}
	if found && rec != nil && rec.RawText != "" {
		p.presenter.OnEvent(model.StepCompletedEvent{StepID: "cache_lookup", Label: "Cache hit", ElapsedMS: 0})
		return rec.RawText, nil
	}
	p.presenter.OnEvent(model.StepCompletedEvent{StepID: "cache_lookup", Label: "Cache miss — fetching", ElapsedMS: 0})

	// Fetch from URL.
	fetchStart := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "fetch", Label: "Fetching JD"})
	text, err := p.fetcher.Fetch(ctx, req.URLOrText)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "fetch", Label: "Fetch failed", Err: err.Error()})
		return "", fmt.Errorf("fetch JD from %s: %w", req.URLOrText, err)
	}
	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "fetch",
		Label:     "JD fetched",
		ElapsedMS: time.Since(fetchStart).Milliseconds(),
	})
	return text, nil
}

// extractKeywords calls the LLM to extract structured JD data from raw text.
// Returns the JDData, a degraded flag (true if extraction failed), and an error.
func (p *ApplyPipeline) extractKeywords(ctx context.Context, jdText string) (model.JDData, bool, error) {
	if p.llm == nil {
		// No orchestrator LLM configured — MCP host handles keyword extraction.
		return model.JDData{}, true, errors.New("no LLM configured: keyword extraction skipped")
	}
	kwStart := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "keywords", Label: "Extracting keywords"})

	jd, err := extractKeywordsFromText(ctx, p.llm, jdText, p.defaults)
	if err != nil {
		p.presenter.OnEvent(model.StepFailedEvent{StepID: "keywords", Label: "Keyword extraction failed", Err: err.Error()})
		return model.JDData{}, true, err
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "keywords",
		Label:     "Keywords extracted",
		ElapsedMS: time.Since(kwStart).Milliseconds(),
	})
	return jd, false, nil
}

// extractKeywordsFromText is a package-level helper used by ApplyPipeline. It calls the LLM with a structured prompt and parses
// the JSON response into a JDData.
func extractKeywordsFromText(ctx context.Context, llmClient port.LLMClient, jdText string, defaults *config.AppDefaults) (model.JDData, error) {
	prompt := fmt.Sprintf(`Extract structured information from the following job description.

Return ONLY a JSON object with these exact keys:
- title (string): job title
- company (string): company name
- required (array of strings): required skills and technologies
- preferred (array of strings): preferred/nice-to-have skills
- location (string): work location
- seniority (string): one of junior, mid, senior, lead, director
- required_years (number): minimum years of experience required

Job Description:
%s`, jdText)

	messages := []model.ChatMessage{
		{Role: "user", Content: prompt},
	}
	opts := model.ChatOptions{
		Temperature: defaults.LLM.KeywordExtractionTemp,
		MaxTokens:   defaults.LLM.KeywordExtractionMaxTokens,
	}

	resp, err := llmClient.ChatComplete(ctx, messages, opts)
	if err != nil {
		return model.JDData{}, fmt.Errorf("llm keyword extraction: %w", err)
	}

	// Raw LLM response type for parsing.
	type rawJD struct {
		Title         string   `json:"title"`
		Company       string   `json:"company"`
		Required      []string `json:"required"`
		Preferred     []string `json:"preferred"`
		Location      string   `json:"location"`
		Seniority     string   `json:"seniority"`
		RequiredYears float64  `json:"required_years"`
	}

	var raw rawJD
	if err := parseJSONFromResponse(resp, &raw); err != nil {
		return model.JDData{}, fmt.Errorf("parse keyword response: %w", err)
	}

	return model.JDData{
		Title:         raw.Title,
		Company:       raw.Company,
		Required:      raw.Required,
		Preferred:     raw.Preferred,
		Location:      raw.Location,
		Seniority:     model.SeniorityLevel(raw.Seniority),
		RequiredYears: raw.RequiredYears,
	}, nil
}

// scoreResumes scores each resume against the JD, returning the full scores map,
// the label of the best resume, and its score.
func (p *ApplyPipeline) scoreResumes(
	ctx context.Context,
	resumeFiles []model.ResumeFile,
	jd *model.JDData,
	cfg *config.Config,
) (map[string]model.ScoreResult, string, float64, error) {
	scoreStart := time.Now()
	p.presenter.OnEvent(model.StepStartedEvent{StepID: "score", Label: "Scoring resumes"})

	scores := make(map[string]model.ScoreResult, len(resumeFiles))
	var bestLabel string
	var bestScore float64

	for _, r := range resumeFiles {
		text, err := p.loader.Load(r.Path)
		if err != nil {
			slog.WarnContext(ctx, "failed to load resume — skipping", "path", r.Path, "error", err)
			continue
		}

		// Augment resume text with profile context before scoring (skipped when Augment is nil).
		augmented := text
		var refData *model.ReferenceData
		if p.augment != nil {
			var augErr error
			augmented, refData, augErr = p.augment.AugmentResumeText(ctx, model.AugmentInput{
				ResumeText: text,
				RefData:    nil,
				JDKeywords: append(jd.Required, jd.Preferred...),
			})
			if augErr != nil {
				slog.WarnContext(ctx, "augmentation failed — using original text", "label", r.Label, "error", augErr)
				augmented = text
				refData = nil
			}
		}

		seniorityMatch := resolveSeniorityMatch(cfg.DefaultSeniority, jd)

		sr, err := p.scorer.Score(&model.ScorerInput{
			ResumeText:     augmented,
			ResumeLabel:    r.Label,
			ResumePath:     r.Path,
			JD:             *jd,
			CandidateYears: cfg.YearsOfExperience,
			RequiredYears:  jd.RequiredYears,
			SeniorityMatch: seniorityMatch,
			ReferenceData:  refData,
		})
		if err != nil {
			slog.WarnContext(ctx, "scoring failed — skipping resume", "label", r.Label, "error", err)
			continue
		}

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
		return nil, "", 0, err
	}

	p.presenter.OnEvent(model.StepCompletedEvent{
		StepID:    "score",
		Label:     "Scoring complete",
		ElapsedMS: time.Since(scoreStart).Milliseconds(),
	})
	return scores, bestLabel, bestScore, nil
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

// profileFromConfig constructs a model.UserProfile from CLI/config-level settings.
func profileFromConfig(cfg *config.Config) model.UserProfile {
	return model.UserProfile{
		Name:              cfg.UserName,
		Occupation:        cfg.Occupation,
		Location:          cfg.Location,
		LinkedInURL:       cfg.LinkedInURL,
		YearsOfExperience: cfg.YearsOfExperience,
		Seniority:         cfg.DefaultSeniority,
	}
}

// runTailorStep finds and loads the best resume, calls the tailor service,
// and re-scores the tailored text. Accomplishments text is passed directly
// via req.AccomplishmentsText — no file loading is performed here.
// It is invoked only when AccomplishmentsText is set and a tailor service is wired.
// resumeFiles is the same slice already scored in Run — passed here to avoid a
// redundant ListResumes call.
func (p *ApplyPipeline) runTailorStep(
	ctx context.Context,
	result *model.PipelineResult,
	jd *model.JDData,
	req ApplyRequest,
	resumeFiles []model.ResumeFile,
) (*model.TailorResult, error) {
	// Find and load the best resume from the already-scored file list.
	var bestFile model.ResumeFile
	for _, r := range resumeFiles {
		if r.Label == result.BestResume {
			bestFile = r
			break
		}
	}
	if bestFile.Path == "" {
		return nil, fmt.Errorf("best resume %q not found in repository", result.BestResume)
	}

	resumeText, err := p.loader.Load(bestFile.Path)
	if err != nil {
		return nil, fmt.Errorf("load best resume for tailor: %w", err)
	}

	scoreBefore := result.Scores[result.BestResume]

	tailorResult, err := p.tailor.TailorResume(ctx, &model.TailorInput{
		Resume:              bestFile,
		ResumeText:          resumeText,
		JD:                  *jd,
		ScoreBefore:         scoreBefore,
		AccomplishmentsText: req.AccomplishmentsText,
		Options: model.TailorOptions{
			MaxTier2BulletRewrites: p.defaults.Tailor.MaxTier2BulletRewrites,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("tailor resume: %w", err)
	}

	// Re-score using tailored text for accurate delta.
	seniorityMatch := resolveSeniorityMatch(req.Config.DefaultSeniority, jd)
	scoreAfter, err := p.scorer.Score(&model.ScorerInput{
		ResumeText:     tailorResult.TailoredText,
		ResumeLabel:    bestFile.Label,
		ResumePath:     bestFile.Path,
		JD:             *jd,
		CandidateYears: req.Config.YearsOfExperience,
		RequiredYears:  jd.RequiredYears,
		SeniorityMatch: seniorityMatch,
	})
	if err != nil {
		return nil, fmt.Errorf("score tailored resume: %w", err)
	}

	tailorResult.NewScore = scoreAfter
	return &tailorResult, nil
}
