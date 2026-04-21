//go:build e2e

package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// newOrchestratorStub returns an httptest.Server that handles POST /chat/completions
// by dispatching on message content to deterministic fixture responses.
//
// Dispatch order (first match wins):
//  1. augment incorporation  — system prompt "You are a resume augmentation assistant"
//     → returns original resume text extracted from <resume_text> tags (no-op augment)
//  2. extract keywords       — "Extract structured information"
//     → reads testdata/llm_responses/extract_keywords.json
//  3. plan T1 skill adds     — "identify which skills from the JD are missing"
//     → inline: {"skill_adds":["observability","OpenTelemetry","microservices"]}
//  4. plan T2 bullet rewrites — "Rewrite relevant Experience bullets"
//     → reads testdata/llm_responses/bullet_rewrite.json
//  5. cover letter           — "Write a professional cover letter"
//     → reads testdata/llm_responses/cover_letter.json
func newOrchestratorStub(t *testing.T) *httptest.Server {
	t.Helper()

	fixtureDir := filepath.Join("testdata", "llm_responses")

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body failed", http.StatusInternalServerError)
			return
		}

		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		allContent := extractMessageContents(req)

		var content string
		switch {
		case strings.Contains(allContent, "You are a resume augmentation assistant"):
			content = extractTaggedText(allContent, "<resume_text>", "</resume_text>")
		case strings.Contains(allContent, "Extract structured information"):
			content = readFixture(t, filepath.Join(fixtureDir, "extract_keywords.json"))
		case strings.Contains(allContent, "identify which skills from the JD are missing"):
			content = `{"skill_adds":["observability","OpenTelemetry","microservices"]}`
		case strings.Contains(allContent, "Rewrite relevant Experience bullets"):
			content = readFixture(t, filepath.Join(fixtureDir, "bullet_rewrite.json"))
		case strings.Contains(allContent, "Write a professional cover letter"):
			content = readFixture(t, filepath.Join(fixtureDir, "cover_letter.json"))
		default:
			content = "{}"
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": content}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Logf("orchestrator stub: encode response: %v", err)
		}
	}))
}

// extractMessageContents joins all message content strings from an OpenAI-format request.
func extractMessageContents(req map[string]any) string {
	msgs, _ := req["messages"].([]any)
	var parts []string
	for _, m := range msgs {
		msg, _ := m.(map[string]any)
		if c, _ := msg["content"].(string); c != "" {
			parts = append(parts, c)
		}
	}
	return strings.Join(parts, "\n")
}

// extractTaggedText returns the trimmed text between open and close tags within content.
// Returns the full content if tags are not found.
func extractTaggedText(content, open, close string) string {
	start := strings.Index(content, open)
	end := strings.Index(content, close)
	if start < 0 || end < 0 || end <= start {
		return content
	}
	return strings.TrimSpace(content[start+len(open) : end])
}
