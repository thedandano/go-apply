// Package orchestrator provides the in-process LLM-backed orchestrator for CLI/TUI mode.
// In MCP mode the MCP host acts as orchestrator; this package is not used there.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// LLMOrchestrator implements port.Orchestrator backed by a generic LLMClient.
// Each method builds a prompt, calls ChatComplete, and parses the JSON response.
type LLMOrchestrator struct {
	client port.LLMClient
}

// Compile-time interface satisfaction check.
var _ port.Orchestrator = (*LLMOrchestrator)(nil)

// NewLLMOrchestrator constructs an LLMOrchestrator with the given LLMClient.
func NewLLMOrchestrator(client port.LLMClient) *LLMOrchestrator {
	return &LLMOrchestrator{client: client}
}

// ExtractKeywords calls the LLM to extract structured JD data from raw text.
func (o *LLMOrchestrator) ExtractKeywords(ctx context.Context, input port.ExtractKeywordsInput) (model.JDData, error) {
	if strings.TrimSpace(input.JDText) == "" {
		return model.JDData{}, fmt.Errorf("jd text is empty — page may not have loaded correctly")
	}

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
%s`, input.JDText)

	messages := []model.ChatMessage{
		{Role: "user", Content: prompt},
	}
	opts := model.ChatOptions{}

	resp, err := o.client.ChatComplete(ctx, messages, opts)
	if err != nil {
		return model.JDData{}, fmt.Errorf("llm keyword extraction: %w", err)
	}

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

// PlanT1 asks the LLM which skills should be injected into the Skills section.
func (o *LLMOrchestrator) PlanT1(ctx context.Context, input *port.PlanT1Input) (port.PlanT1Output, error) {
	allKeywords := append(input.JDData.Required, input.JDData.Preferred...) //nolint:gocritic // fresh slice intentional
	prompt := fmt.Sprintf(`You are a resume tailoring assistant.

Given the job description keywords and the candidate's resume, identify which skills from the JD are missing from the resume's Skills section and should be added.

JD Keywords: %s

Current Resume:
%s

Skills Reference:
%s

Return ONLY a JSON object with this key:
- skill_adds (array of strings): list of skills to inject into the Skills section (empty array if none needed)`,
		strings.Join(allKeywords, ", "),
		input.ResumeText,
		input.SkillsRef,
	)

	messages := []model.ChatMessage{
		{Role: "user", Content: prompt},
	}
	opts := model.ChatOptions{}

	resp, err := o.client.ChatComplete(ctx, messages, opts)
	if err != nil {
		return port.PlanT1Output{}, fmt.Errorf("llm plan t1: %w", err)
	}

	type rawT1 struct {
		SkillAdds []string `json:"skill_adds"`
	}

	var raw rawT1
	if err := parseJSONFromResponse(resp, &raw); err != nil {
		return port.PlanT1Output{}, fmt.Errorf("parse plan t1 response: %w", err)
	}

	return port.PlanT1Output{SkillAdds: raw.SkillAdds}, nil
}

// PlanT2 asks the LLM for bullet rewrites grounded in accomplishments.
func (o *LLMOrchestrator) PlanT2(ctx context.Context, input *port.PlanT2Input) (port.PlanT2Output, error) {
	allKeywords := append(input.JDData.Required, input.JDData.Preferred...) //nolint:gocritic // fresh slice intentional
	prompt := fmt.Sprintf(`You are a resume tailoring assistant.

Rewrite relevant Experience bullets from the resume to better highlight impact and relevance to the JD keywords.
Ground rewrites in the candidate's accomplishments below.

JD Keywords: %s

Resume:
%s

Candidate Accomplishments:
%s

Return ONLY a JSON object with this key:
- rewrites (array of objects): each object has "original" (exact bullet text) and "replacement" (rewritten bullet)
  Return an empty array if no rewrites are needed.`,
		strings.Join(allKeywords, ", "),
		input.ResumeText,
		input.Accomplishments,
	)

	messages := []model.ChatMessage{
		{Role: "user", Content: prompt},
	}
	opts := model.ChatOptions{}

	resp, err := o.client.ChatComplete(ctx, messages, opts)
	if err != nil {
		return port.PlanT2Output{}, fmt.Errorf("llm plan t2: %w", err)
	}

	type rawT2 struct {
		Rewrites []port.BulletRewrite `json:"rewrites"`
	}

	var raw rawT2
	if err := parseJSONFromResponse(resp, &raw); err != nil {
		return port.PlanT2Output{}, fmt.Errorf("parse plan t2 response: %w", err)
	}

	return port.PlanT2Output{Rewrites: raw.Rewrites}, nil
}

// GenerateCoverLetter asks the LLM to write a cover letter and returns the raw text.
func (o *LLMOrchestrator) GenerateCoverLetter(ctx context.Context, input *port.CoverLetterInput) (string, error) {
	prompt := fmt.Sprintf(`Write a professional cover letter for %s applying to the %s role at %s.

Resume summary:
%s

Keep the cover letter concise (3–4 paragraphs), results-driven, and tailored to the job.
Return ONLY the cover letter text — no subject line, no metadata.`,
		input.CandidateName,
		input.JDData.Title,
		input.JDData.Company,
		input.ResumeText,
	)

	messages := []model.ChatMessage{
		{Role: "user", Content: prompt},
	}
	opts := model.ChatOptions{}

	resp, err := o.client.ChatComplete(ctx, messages, opts)
	if err != nil {
		return "", fmt.Errorf("llm generate cover letter: %w", err)
	}

	return strings.TrimSpace(resp), nil
}

// jsonBlockRe matches a JSON object inside a markdown code fence (```json or ```).
var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\})\\s*```")

// parseJSONFromResponse extracts and parses the first JSON object from an LLM response.
// It handles three common formats:
//  1. Raw JSON: {"key": "value"}
//  2. Markdown-fenced: ```json\n{"key": "value"}\n```
//  3. JSON embedded in prose: "Here is the result: {"key": "value"}"
func parseJSONFromResponse(resp string, v any) error {
	if m := jsonBlockRe.FindStringSubmatch(resp); len(m) > 1 {
		resp = m[1]
	}

	start := -1
	for i, c := range resp {
		if c == '{' {
			start = i
			break
		}
	}
	if start == -1 {
		return fmt.Errorf("no JSON object found in response")
	}

	end := -1
	for i := len(resp) - 1; i >= 0; i-- {
		if resp[i] == '}' {
			end = i
			break
		}
	}
	if end == -1 {
		return fmt.Errorf("no closing brace found in response")
	}

	return json.Unmarshal([]byte(resp[start:end+1]), v)
}
