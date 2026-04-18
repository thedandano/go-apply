package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/repository/fs"
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

	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "onboard_user"),
		slog.String("resume_label", resumeLabel),
		slog.Int("resume_content_len", len(resumeContent)),
		slog.Int("skills_len", len(skills)),
		slog.Int("accomplishments_len", len(accomplishments)),
		logger.PayloadAttr("resume_content", resumeContent, logger.Verbose()),
	)

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
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "onboard_user"),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(data)),
		logger.PayloadAttr("result", string(data), logger.Verbose()),
	)
	return mcp.NewToolResultText(string(data))
}

// HandleAddResume is the exported, injectable handler for "add_resume".
// Both resume_content and resume_label are required.
func HandleAddResume(ctx context.Context, req *mcp.CallToolRequest, svc port.Onboarder) *mcp.CallToolResult {
	resumeContent := req.GetString("resume_content", "")
	resumeLabel := req.GetString("resume_label", "")

	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "add_resume"),
		slog.String("resume_label", resumeLabel),
		slog.Int("resume_content_len", len(resumeContent)),
		logger.PayloadAttr("resume_content", resumeContent, logger.Verbose()),
	)

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
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "add_resume"),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(data)),
		logger.PayloadAttr("result", string(data), logger.Verbose()),
	)
	return mcp.NewToolResultText(string(data))
}

// HandleUpdateConfig is the exported, injectable handler for "update_config".
// cfg is loaded and saved by the caller.
func HandleUpdateConfig(ctx context.Context, req *mcp.CallToolRequest, cfg *config.Config) *mcp.CallToolResult {
	key := req.GetString("key", "")
	value := req.GetString("value", "")

	displayValue := value
	if config.IsAPIKey(key) && value != "" {
		displayValue = "***"
	}
	slog.DebugContext(ctx, "mcp tool invoked",
		slog.String("tool", "update_config"),
		slog.String("key", key),
		slog.String("value", displayValue),
	)

	if key == "" {
		return errorResult("key is required")
	}
	if strings.HasPrefix(key, "orchestrator.") {
		return errorResult("orchestrator config is not used in MCP mode: Claude is the orchestrator. To configure for CLI/TUI use, edit ~/.config/go-apply/config.yaml directly.")
	}
	if err := cfg.SetField(key, value); err != nil {
		return errorResult(err.Error())
	}
	if err := cfg.Save(); err != nil {
		return errorResult(fmt.Sprintf("save config: %v", err))
	}
	initRedactor()
	data, _ := json.Marshal(map[string]string{"updated": key, "value": displayValue})
	slog.DebugContext(ctx, "mcp tool result",
		slog.String("tool", "update_config"),
		slog.String("status", "ok"),
		slog.String("key", key),
		slog.String("value", displayValue),
	)
	return mcp.NewToolResultText(string(data))
}

// HandleGetConfigWith renders the config as redacted JSON. Exported for testing.
// Only MCP-relevant keys are included; orchestrator keys are excluded because
// in MCP mode Claude is the orchestrator.
func HandleGetConfigWith(cfg *config.Config) *mcp.CallToolResult {
	return HandleGetConfigWithProfileAndFiles(cfg, config.DataDir())
}

// HandleGetConfigWithProfileAndFiles renders the config plus profile status as redacted JSON.
// This is the full implementation; HandleGetConfigWith is a convenience wrapper.
// Only MCP-relevant keys are included; orchestrator keys are excluded because
// in MCP mode Claude is the orchestrator.
// Exported for testing.
func HandleGetConfigWithProfileAndFiles(cfg *config.Config, dataDir string) *mcp.CallToolResult {
	slog.Debug("mcp tool invoked", slog.String("tool", "get_config"))
	response := make(map[string]interface{}, len(config.MCPKeys())+1)

	// Add config fields
	fields := make(map[string]string, len(config.MCPKeys()))
	for _, key := range config.MCPKeys() {
		value, _ := cfg.GetField(key)
		if config.IsAPIKey(key) && value != "" {
			value = "***"
		}
		fields[key] = value
	}

	// Merge config fields into response
	for k, v := range fields {
		response[k] = v
	}

	// Derive profile status
	profileObj := buildProfileStatus(dataDir)
	response["profile"] = profileObj

	data, _ := json.Marshal(response)
	slog.Debug("mcp tool result",
		slog.String("tool", "get_config"),
		slog.String("status", "ok"),
		slog.Int("result_bytes", len(data)),
		logger.PayloadAttr("result", string(data), logger.Verbose()),
	)
	return mcp.NewToolResultText(string(data))
}

// buildProfileStatus derives onboarding status from the data directory.
// Returns a map with:
//   - onboarded (bool): true if at least one resume exists
//   - resumes ([]string): list of resume labels found
//   - has_skills (bool): true if skills.md exists and is non-empty
//   - has_accomplishments (bool): true if accomplishments.md exists and is non-empty
func buildProfileStatus(dataDir string) map[string]interface{} {
	profileObj := map[string]interface{}{
		"onboarded":           false,
		"resumes":             []string{},
		"has_skills":          false,
		"has_accomplishments": false,
	}

	// List resumes
	resumeRepo := fs.NewResumeRepository(dataDir)
	resumes, err := resumeRepo.ListResumes()
	if err == nil && len(resumes) > 0 {
		profileObj["onboarded"] = true
		resumeLabels := make([]string, len(resumes))
		for i, r := range resumes {
			resumeLabels[i] = r.Label
		}
		profileObj["resumes"] = resumeLabels
	}

	// Check for skills.md
	skillsPath := filepath.Join(dataDir, "inputs", "skills.md")
	if info, err := os.Stat(skillsPath); err == nil && info.Size() > 0 {
		profileObj["has_skills"] = true
	}

	// Check for accomplishments.md
	accomplishmentsPath := filepath.Join(dataDir, "inputs", "accomplishments.md")
	if info, err := os.Stat(accomplishmentsPath); err == nil && info.Size() > 0 {
		profileObj["has_accomplishments"] = true
	}

	return profileObj
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
	cfg.ApplyOverrides(defaults)

	if strings.TrimSpace(cfg.Embedder.BaseURL) == "" || strings.TrimSpace(cfg.Embedder.Model) == "" {
		return nil, nil, fmt.Errorf("embedder not configured — use update_config to set embedder.base_url, embedder.model, and optionally embedder.api_key before onboarding")
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
