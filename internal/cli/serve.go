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
		RunE: func(cmd *cobra.Command, _ []string) error {
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
