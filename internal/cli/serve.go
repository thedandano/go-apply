package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/thedandano/go-apply/internal/config"
	loaderPkg "github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	mcpPres "github.com/thedandano/go-apply/internal/presenter/mcp"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/augment"
	"github.com/thedandano/go-apply/internal/service/coverletter"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/onboarding"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
)

func newServeCommand(defaults *config.AppDefaults) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start MCP stdio server for AI agent integration",
		Long: `serve starts an MCP stdio server that exposes three tools:
  apply_to_job   — full apply pipeline (score + cover letter)
  get_score      — score resumes against a JD and return detailed scores
  tailor_resume  — tailor resume (stub — requires Task 13)`,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			llmClient := newLLMClient(cfg, defaults)
			embedderClient := newEmbedderClient(cfg, defaults)

			var augmenterSvc port.Augmenter
			profileRepo, dbErr := newSQLiteProfile(cfg, defaults)
			if dbErr == nil {
				augmenterSvc = augment.New(profileRepo, embedderClient, defaults)
			}

			scorerSvc := scorer.New(defaults)
			clGen := coverletter.New(llmClient, defaults)
			fetcherSvc := fetcher.NewFallbackFetcher(defaults)
			docLoader := loaderPkg.New()

			// buildPipeline creates a pipeline with a fresh MCP presenter for the given resume dir.
			buildPipeline := func(resumeDir string) (*pipeline.ApplyPipeline, *mcpPres.Presenter) {
				pres := mcpPres.New()
				cacheDir := filepath.Join(config.DataDir(), "jd_cache")
				jdCacheRepo := fs.NewJDCacheRepository(cacheDir)
				p := pipeline.New(pipeline.Config{
					Fetcher:   fetcherSvc,
					LLM:       llmClient,
					Scorer:    scorerSvc,
					CLGen:     clGen,
					Resumes:   fs.NewResumeRepository(resumeDir),
					JDCache:   jdCacheRepo,
					Augmenter: augmenterSvc,
					DocLoader: docLoader,
					Presenter: pres,
					Defaults:  defaults,
					Cfg:       cfg,
				})
				return p, pres
			}

			s := server.NewMCPServer("go-apply", "0.1.0",
				server.WithToolCapabilities(true),
			)

			// ── apply_to_job ──────────────────────────────────────────────────
			s.AddTool(
				mcp.NewTool("apply_to_job",
					mcp.WithDescription("Score resumes against a job description and generate a cover letter"),
					mcp.WithString("jd_url",
						mcp.Description("Job description URL (mutually exclusive with jd_text)"),
					),
					mcp.WithString("jd_text",
						mcp.Description("Raw job description text (mutually exclusive with jd_url)"),
					),
					mcp.WithString("channel",
						mcp.Description("Application channel: COLD, REFERRAL, or RECRUITER (default: COLD)"),
					),
					mcp.WithString("resume_dir",
						mcp.Description("Directory to scan for resumes (default: current directory)"),
					),
				),
				func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return handleApplyToJob(ctx, req, buildPipeline)
				},
			)

			// ── get_score ─────────────────────────────────────────────────────
			s.AddTool(
				mcp.NewTool("get_score",
					mcp.WithDescription("Score resumes against a job description and return detailed scores"),
					mcp.WithString("jd_url",
						mcp.Description("Job description URL (mutually exclusive with jd_text)"),
					),
					mcp.WithString("jd_text",
						mcp.Description("Raw job description text (mutually exclusive with jd_url)"),
					),
					mcp.WithString("resume_dir",
						mcp.Description("Directory to scan for resumes (default: current directory)"),
					),
				),
				func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return handleGetScore(ctx, req, buildPipeline)
				},
			)

			// ── tailor_resume (stub) ──────────────────────────────────────────
			s.AddTool(
				mcp.NewTool("tailor_resume",
					mcp.WithDescription("Tailor resume to a job description (stub — requires Task 13)"),
					mcp.WithString("jd_url", mcp.Description("Job description URL")),
					mcp.WithString("jd_text", mcp.Description("Raw job description text")),
					mcp.WithString("resume_path", mcp.Description("Path to the resume file")),
				),
				func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return mcp.NewToolResultText(`{"error":"tailor_resume not yet implemented — requires Task 13"}`), nil
				},
			)

			// ── onboard_user ──────────────────────────────────────────────────
			s.AddTool(
				mcp.NewTool("onboard_user",
					mcp.WithDescription("Index a resume and optional skills/accomplishments into the profile database"),
					mcp.WithString("resume_content", mcp.Description("Plain text of the resume")),
					mcp.WithString("resume_label", mcp.Description("Label for the resume (e.g. 'backend')")),
					mcp.WithString("resume_format", mcp.Description("File format of the resume (e.g. '.pdf')")),
					mcp.WithString("skills_content", mcp.Description("Plain text of the skills reference document (optional)")),
					mcp.WithString("accomplishments_content", mcp.Description("Plain text of the accomplishments document (optional)")),
				),
				func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return handleOnboardUser(ctx, req, defaults)
				},
			)

			// ── add_resume ────────────────────────────────────────────────────
			s.AddTool(
				mcp.NewTool("add_resume",
					mcp.WithDescription("Index a single resume into the profile database"),
					mcp.WithString("content", mcp.Description("Plain text of the resume")),
					mcp.WithString("label", mcp.Description("Label for the resume (e.g. 'backend')")),
					mcp.WithString("format", mcp.Description("File format of the resume (e.g. '.pdf')")),
				),
				func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return handleAddResume(ctx, req, defaults)
				},
			)

			// ── update_config ─────────────────────────────────────────────────
			s.AddTool(
				mcp.NewTool("update_config",
					mcp.WithDescription("Set a go-apply config field by dot-notation key"),
					mcp.WithString("key", mcp.Description("Config key (e.g. 'orchestrator.model')")),
					mcp.WithString("value", mcp.Description("Value to set")),
				),
				func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return handleUpdateConfig(req)
				},
			)

			// ── get_config ────────────────────────────────────────────────────
			s.AddTool(
				mcp.NewTool("get_config",
					mcp.WithDescription("Get go-apply config field(s). Omit key to return full config."),
					mcp.WithString("key", mcp.Description("Config key (optional; empty = return all)")),
				),
				func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return handleGetConfig(req)
				},
			)

			return server.ServeStdio(s)
		},
	}
	return cmd
}

type pipelineBuilder func(resumeDir string) (*pipeline.ApplyPipeline, *mcpPres.Presenter)

// handleApplyToJob handles the apply_to_job MCP tool.
// Validates inputs, runs the pipeline, and returns the result as JSON.
// Business logic errors are returned as JSON error objects, never as Go errors.
func handleApplyToJob(ctx context.Context, req mcp.CallToolRequest, build pipelineBuilder) (*mcp.CallToolResult, error) {
	jdURL := req.GetString("jd_url", "")
	jdText := req.GetString("jd_text", "")
	channel := req.GetString("channel", "COLD")
	resumeDir := req.GetString("resume_dir", ".")

	if jdURL == "" && jdText == "" {
		return mcp.NewToolResultText(`{"error":"exactly one of jd_url or jd_text must be provided"}`), nil
	}
	if jdURL != "" && jdText != "" {
		return mcp.NewToolResultText(`{"error":"jd_url and jd_text are mutually exclusive"}`), nil
	}

	switch model.ChannelType(channel) {
	case model.ChannelCold, model.ChannelReferral, model.ChannelRecruiter:
		// valid
	default:
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":"invalid channel %q: must be COLD, REFERRAL, or RECRUITER"}`, channel)), nil
	}

	p, pres := build(resumeDir)

	if runErr := p.Run(ctx, pipeline.RunInput{
		URL:     jdURL,
		Text:    jdText,
		Channel: model.ChannelType(channel),
	}); runErr != nil && pres.Err() == nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, runErr.Error())), nil
	}

	if presErr := pres.Err(); presErr != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, presErr.Error())), nil
	}

	data, err := json.Marshal(pres.Result())
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// handleOnboardUser handles the onboard_user MCP tool.
// Accepts pre-extracted text for resume, skills, and accomplishments.
func handleOnboardUser(ctx context.Context, req mcp.CallToolRequest, defaults *config.AppDefaults) (*mcp.CallToolResult, error) {
	resumeContent := req.GetString("resume_content", "")
	resumeLabel := req.GetString("resume_label", "")
	resumeFormat := req.GetString("resume_format", "")
	skillsContent := req.GetString("skills_content", "")
	accomplishmentsContent := req.GetString("accomplishments_content", "")

	cfg, err := config.Load()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	profileRepo, err := newSQLiteProfile(cfg, defaults)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	embedderClient := newEmbedderClient(cfg, defaults)
	dataDir := config.DataDir()
	svc := onboarding.New(profileRepo, embedderClient, dataDir)

	input := onboarding.OnboardInput{
		Resumes:             make(map[string]onboarding.OnboardFile),
		SkillsText:          skillsContent,
		AccomplishmentsText: accomplishmentsContent,
	}

	if (resumeContent == "") != (resumeLabel == "") {
		return mcp.NewToolResultText(`{"error":"resume_content and resume_label must both be provided or both omitted"}`), nil
	}

	if resumeContent != "" && resumeLabel != "" {
		input.Resumes[resumeLabel] = onboarding.OnboardFile{
			Label:     resumeLabel,
			PlainText: resumeContent,
			Format:    resumeFormat,
		}
	}

	result, err := svc.Run(ctx, input)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// handleAddResume handles the add_resume MCP tool.
// Indexes a single resume into the profile database.
func handleAddResume(ctx context.Context, req mcp.CallToolRequest, defaults *config.AppDefaults) (*mcp.CallToolResult, error) {
	content := req.GetString("content", "")
	label := req.GetString("label", "")
	format := req.GetString("format", "")

	if content == "" || label == "" {
		return mcp.NewToolResultText(`{"error":"content and label are required"}`), nil
	}

	cfg, err := config.Load()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	profileRepo, err := newSQLiteProfile(cfg, defaults)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	embedderClient := newEmbedderClient(cfg, defaults)
	dataDir := config.DataDir()
	svc := onboarding.New(profileRepo, embedderClient, dataDir)

	input := onboarding.OnboardInput{
		Resumes: map[string]onboarding.OnboardFile{
			label: {
				Label:     label,
				PlainText: content,
				Format:    format,
			},
		},
	}

	result, err := svc.Run(ctx, input)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// handleUpdateConfig handles the update_config MCP tool.
func handleUpdateConfig(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key := req.GetString("key", "")
	value := req.GetString("value", "")

	if key == "" {
		return mcp.NewToolResultText(`{"error":"key is required"}`), nil
	}

	cfg, err := config.Load()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	if err := cfg.SetField(key, value); err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	if err := cfg.Save(); err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	return mcp.NewToolResultText(`{"ok":true}`), nil
}

// handleGetConfig handles the get_config MCP tool.
// If key is empty, returns the full redacted config. Otherwise returns the field value.
func handleGetConfig(req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key := req.GetString("key", "")

	cfg, err := config.Load()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}

	if key != "" {
		val, err := cfg.GetField(key)
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
		}
		if isAPIKey(key) && val != "" {
			val = "[redacted]"
		}
		data, _ := json.Marshal(map[string]string{"key": key, "value": val})
		return mcp.NewToolResultText(string(data)), nil
	}

	// Return full config with API keys redacted
	redacted := *cfg
	if redacted.Orchestrator.APIKey != "" {
		redacted.Orchestrator.APIKey = "[redacted]"
	}
	if redacted.Embedder.APIKey != "" {
		redacted.Embedder.APIKey = "[redacted]"
	}

	data, err := json.Marshal(redacted)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// handleGetScore handles the get_score MCP tool.
// Runs the full pipeline and returns result JSON (cover letter is included but callers may ignore it).
// Business logic errors are returned as JSON error objects, never as Go errors.
func handleGetScore(ctx context.Context, req mcp.CallToolRequest, build pipelineBuilder) (*mcp.CallToolResult, error) {
	jdURL := req.GetString("jd_url", "")
	jdText := req.GetString("jd_text", "")
	resumeDir := req.GetString("resume_dir", ".")

	if jdURL == "" && jdText == "" {
		return mcp.NewToolResultText(`{"error":"exactly one of jd_url or jd_text must be provided"}`), nil
	}
	if jdURL != "" && jdText != "" {
		return mcp.NewToolResultText(`{"error":"jd_url and jd_text are mutually exclusive"}`), nil
	}

	p, pres := build(resumeDir)

	if runErr := p.Run(ctx, pipeline.RunInput{
		URL:     jdURL,
		Text:    jdText,
		Channel: model.ChannelCold,
	}); runErr != nil && pres.Err() == nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, runErr.Error())), nil
	}

	if presErr := pres.Err(); presErr != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, presErr.Error())), nil
	}

	data, err := json.Marshal(pres.Result())
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf(`{"error":%q}`, err.Error())), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}
