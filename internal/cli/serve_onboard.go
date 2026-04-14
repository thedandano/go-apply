package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/llm"
	"github.com/thedandano/go-apply/internal/service/onboarding"
)

// handleOnboardUser is the MCP handler for "onboard_user".
// It accepts a resume (content + label), skills, and accomplishments as raw text.
// Config is loaded fresh per invocation. No Go error is returned; failures become JSON error results.
func handleOnboardUser(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resumeContent := req.GetString("resume_content", "")
	resumeLabel := req.GetString("resume_label", "")
	skills := req.GetString("skills", "")
	accomplishments := req.GetString("accomplishments", "")

	// XOR validation: both or neither.
	if (resumeContent == "") != (resumeLabel == "") {
		return errorResult("resume_content and resume_label must both be provided or both omitted"), nil
	}
	if resumeContent == "" && skills == "" && accomplishments == "" {
		return errorResult("at least one of resume_content, skills, or accomplishments is required"), nil
	}

	svc, err := newOnboardingService()
	if err != nil {
		return errorResult(fmt.Sprintf("setup: %v", err)), nil
	}

	var resumes []model.ResumeEntry
	if resumeContent != "" {
		resumes = append(resumes, model.ResumeEntry{Label: resumeLabel, Text: resumeContent})
	}

	result, runErr := svc.Run(ctx, model.OnboardInput{
		Resumes:             resumes,
		SkillsText:          skills,
		AccomplishmentsText: accomplishments,
	})
	if runErr != nil {
		return errorResult(fmt.Sprintf("onboard: %v", runErr)), nil
	}

	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

// handleAddResume is the MCP handler for "add_resume".
// Both resume_content and resume_label are required.
func handleAddResume(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resumeContent := req.GetString("resume_content", "")
	resumeLabel := req.GetString("resume_label", "")

	if resumeContent == "" || resumeLabel == "" {
		return errorResult("resume_content and resume_label are both required"), nil
	}

	svc, err := newOnboardingService()
	if err != nil {
		return errorResult(fmt.Sprintf("setup: %v", err)), nil
	}

	result, err := svc.Run(ctx, model.OnboardInput{
		Resumes: []model.ResumeEntry{{Label: resumeLabel, Text: resumeContent}},
	})
	if err != nil {
		return errorResult(fmt.Sprintf("add resume: %v", err)), nil
	}

	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

// handleUpdateConfig is the MCP handler for "update_config".
// Loads config fresh, calls SetField, and saves.
func handleUpdateConfig(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key := req.GetString("key", "")
	value := req.GetString("value", "")
	if key == "" {
		return errorResult("key is required"), nil
	}

	cfg, err := config.Load()
	if err != nil {
		return errorResult(fmt.Sprintf("load config: %v", err)), nil
	}
	if err := cfg.SetField(key, value); err != nil {
		return errorResult(err.Error()), nil
	}
	if err := cfg.Save(); err != nil {
		return errorResult(fmt.Sprintf("save config: %v", err)), nil
	}

	data, _ := json.Marshal(map[string]string{"updated": key, "value": value})
	return mcp.NewToolResultText(string(data)), nil
}

// handleGetConfig is the MCP handler for "get_config".
// Returns all config fields with API keys redacted.
func handleGetConfig(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return errorResult(fmt.Sprintf("load config: %v", err)), nil
	}

	fields := make(map[string]string, len(config.AllKeys()))
	for _, key := range config.AllKeys() {
		value, _ := cfg.GetField(key)
		if config.IsAPIKey(key) && value != "" {
			value = "***"
		}
		fields[key] = value
	}

	data, _ := json.Marshal(fields)
	return mcp.NewToolResultText(string(data)), nil
}

// newOnboardingService wires the onboarding service with a fresh config load.
func newOnboardingService() (*onboarding.Service, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	defaults, err := config.LoadDefaults()
	if err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}

	log := slog.Default()
	embedderClient := llm.New(cfg.Embedder.BaseURL, cfg.Embedder.Model, cfg.Embedder.APIKey, defaults, log)
	profileRepo, err := newSQLiteProfile(cfg)
	if err != nil {
		return nil, err
	}

	return onboarding.New(profileRepo, embedderClient, config.DataDir(), log), nil
}
