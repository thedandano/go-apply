package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/storycreator"
)

// HandleCreateStory is the MCP-wired handler for the create_story tool.
func HandleCreateStory(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	dataDir := config.DataDir()
	creator := storycreator.New(dataDir, fs.NewCareerRepository(), slog.Default())
	return HandleCreateStoryWith(ctx, req.GetArguments(), creator)
}

// HandleCreateStoryWith is the testable core — accepts parsed args and a StoryCreatorService.
// After saving the story it returns needs_compile: true so the host triggers recompilation.
func HandleCreateStoryWith(
	ctx context.Context,
	args map[string]interface{},
	creator port.StoryCreatorService,
) *mcp.CallToolResult {
	input, err := parseStoryArgs(args)
	if err != nil {
		return errorResult(err.Error())
	}

	out, err := creator.Create(ctx, input)
	if err != nil {
		return errorResult(err.Error())
	}

	resp := map[string]interface{}{
		"story_id":      out.StoryID,
		"needs_compile": true,
	}
	data, marshalErr := json.Marshal(resp)
	if marshalErr != nil {
		return errorResult("marshal response: " + marshalErr.Error())
	}
	return mcp.NewToolResultText(string(data))
}

// parseStoryArgs extracts and validates required create_story arguments.
func parseStoryArgs(args map[string]interface{}) (model.StoryInput, error) {
	skill, _ := args["skill"].(string)
	storyTypeRaw, _ := args["story_type"].(string)
	jobTitle, _ := args["job_title"].(string)
	situation, _ := args["situation"].(string)
	behavior, _ := args["behavior"].(string)
	impact, _ := args["impact"].(string)

	if strings.TrimSpace(skill) == "" {
		return model.StoryInput{}, fmt.Errorf("skill is required")
	}
	if strings.TrimSpace(jobTitle) == "" {
		return model.StoryInput{}, fmt.Errorf("job_title is required")
	}
	if strings.TrimSpace(situation) == "" {
		return model.StoryInput{}, fmt.Errorf("empty field: situation")
	}
	if strings.TrimSpace(behavior) == "" {
		return model.StoryInput{}, fmt.Errorf("empty field: behavior")
	}
	if strings.TrimSpace(impact) == "" {
		return model.StoryInput{}, fmt.Errorf("empty field: impact")
	}

	storyType, err := parseStoryType(storyTypeRaw)
	if err != nil {
		return model.StoryInput{}, err
	}

	isNewJob, _ := args["is_new_job"].(bool)
	startDate, _ := args["job_start_date"].(string)
	endDate, _ := args["job_end_date"].(string)

	if isNewJob && (strings.TrimSpace(startDate) == "" || strings.TrimSpace(endDate) == "") {
		return model.StoryInput{}, fmt.Errorf("new job missing dates: job_start_date and job_end_date required when is_new_job=true")
	}

	return model.StoryInput{
		PrimarySkill: skill,
		StoryType:    storyType,
		JobTitle:     jobTitle,
		IsNewJob:     isNewJob,
		StartDate:    startDate,
		EndDate:      endDate,
		Situation:    situation,
		Behavior:     behavior,
		Impact:       impact,
	}, nil
}

// parseStoryType validates and converts the raw story_type string.
func parseStoryType(raw string) (model.StoryType, error) {
	switch model.StoryType(raw) {
	case model.StoryTypeProject, model.StoryTypeAchievement, model.StoryTypeTechnical,
		model.StoryTypeLeadership, model.StoryTypeProcess, model.StoryTypeCollaboration:
		return model.StoryType(raw), nil
	default:
		return "", fmt.Errorf("invalid story_type %q: must be project, achievement, technical, leadership, process, or collaboration", raw)
	}
}
