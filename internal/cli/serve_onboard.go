package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/onboarding"
)

// HandleOnboardUser is the exported, injectable handler for "onboard_user".
// svc must not be nil. This function never returns a Go error; failures become JSON error results.
func HandleOnboardUser(ctx context.Context, req *mcp.CallToolRequest, svc port.Onboarder) *mcp.CallToolResult {
	resumeContent := req.GetString("resume_content", "")
	resumeLabel := req.GetString("resume_label", "")
	skills := req.GetString("skills", "")
	accomplishments := req.GetString("accomplishments", "")

	// XOR validation: both or neither.
	if (resumeContent == "") != (resumeLabel == "") {
		return errorResult("resume_content and resume_label must both be provided or both omitted")
	}
	if resumeContent == "" {
		return errorResult("resume is required")
	}

	var resumes []model.ResumeEntry
	if resumeContent != "" {
		resumes = append(resumes, model.ResumeEntry{Label: resumeLabel, Text: resumeContent})
	}

	result, err := svc.Run(ctx, model.OnboardInput{
		Resumes:             resumes,
		SkillsText:          skills,
		AccomplishmentsText: accomplishments,
	})
	if err != nil {
		return errorResult(fmt.Sprintf("onboard: %v", err))
	}

	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data))
}

// HandleAddResume is the exported, injectable handler for "add_resume".
// Both resume_content and resume_label are required.
func HandleAddResume(ctx context.Context, req *mcp.CallToolRequest, svc port.Onboarder) *mcp.CallToolResult {
	resumeContent := req.GetString("resume_content", "")
	resumeLabel := req.GetString("resume_label", "")

	if resumeContent == "" || resumeLabel == "" {
		return errorResult("resume_content and resume_label are both required")
	}

	result, err := svc.Run(ctx, model.OnboardInput{
		Resumes: []model.ResumeEntry{{Label: resumeLabel, Text: resumeContent}},
	})
	if err != nil {
		return errorResult(fmt.Sprintf("add resume: %v", err))
	}

	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data))
}

// HandleUpdateConfig is the exported, injectable handler for "update_config".
// cfg is loaded and saved by the caller.
func HandleUpdateConfig(_ context.Context, req *mcp.CallToolRequest, cfg *config.Config) *mcp.CallToolResult {
	key := req.GetString("key", "")
	value := req.GetString("value", "")
	if key == "" {
		return errorResult("key is required")
	}
	if err := cfg.SetField(key, value); err != nil {
		return errorResult(err.Error())
	}
	if err := cfg.Save(); err != nil {
		return errorResult(fmt.Sprintf("save config: %v", err))
	}
	displayValue := value
	if config.IsAPIKey(key) && value != "" {
		displayValue = "***"
	}
	data, _ := json.Marshal(map[string]string{"updated": key, "value": displayValue})
	return mcp.NewToolResultText(string(data))
}

// HandleGetConfigWith renders the config as redacted JSON. Exported for testing.
func HandleGetConfigWith(cfg *config.Config) *mcp.CallToolResult {
	fields := make(map[string]string, len(config.AllKeys()))
	for _, key := range config.AllKeys() {
		value, _ := cfg.GetField(key)
		if config.IsAPIKey(key) && value != "" {
			value = "***"
		}
		fields[key] = value
	}
	data, _ := json.Marshal(fields)
	return mcp.NewToolResultText(string(data))
}

// newOnboardSvc opens a fresh SQLite profile repository and constructs an
// onboarding.Service. The returned cleanup function must be called when the
// service is no longer needed to release the database connection.
func newOnboardSvc() (port.Onboarder, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}
	defaults, err := config.LoadDefaults()
	if err != nil {
		return nil, nil, fmt.Errorf("load defaults: %w", err)
	}

	log := slog.Default()
	embedderClient := llm.New(cfg.Embedder.BaseURL, cfg.Embedder.Model, cfg.Embedder.APIKey, defaults, log)
	profileRepo, err := newSQLiteProfile(cfg)
	if err != nil {
		return nil, nil, err
	}

	svc := onboarding.New(profileRepo, embedderClient, config.DataDir(), log)
	cleanup := func() { _ = profileRepo.Close() }
	return svc, cleanup, nil
}
