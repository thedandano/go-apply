package tailor

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
)

// stubLLMClient returns a fixed response for each ChatComplete call.
type stubLLMClient struct {
	response string
	err      error
}

func (s *stubLLMClient) ChatComplete(_ context.Context, _ []model.ChatMessage, _ model.ChatOptions) (string, error) {
	return s.response, s.err
}

func TestExtractExperienceBullets(t *testing.T) {
	resume := `## Experience

- Led a team of 5 engineers
• Delivered 3 major features under budget
* Reduced latency by 40%

## Skills
Python, Golang
`
	bullets := extractExperienceBullets(resume)

	if len(bullets) != 3 {
		t.Fatalf("expected 3 bullets, got %d: %v", len(bullets), bullets)
	}
	// Verify first bullet is preserved with its original marker.
	if !strings.HasPrefix(strings.TrimSpace(bullets[0].Line), "-") {
		t.Errorf("expected first bullet to start with '-', got %q", bullets[0].Line)
	}
	if !strings.HasPrefix(strings.TrimSpace(bullets[1].Line), "•") {
		t.Errorf("expected second bullet to start with '•', got %q", bullets[1].Line)
	}
	if !strings.HasPrefix(strings.TrimSpace(bullets[2].Line), "*") {
		t.Errorf("expected third bullet to start with '*', got %q", bullets[2].Line)
	}
	// Verify that indices are strictly increasing and in range.
	if bullets[0].Index >= bullets[1].Index || bullets[1].Index >= bullets[2].Index {
		t.Errorf("expected strictly increasing indices, got %d, %d, %d",
			bullets[0].Index, bullets[1].Index, bullets[2].Index)
	}
}

func TestRewriteBullets_Tier2HappyPath(t *testing.T) {
	resume := `## Experience

- Led a team of 5 engineers building Kubernetes clusters

## Skills
Python
`
	defaults, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	defaults.Tailor.MaxTier2BulletRewrites = 5

	stub := &stubLLMClient{response: "- Led a cross-functional team of 5 engineers deploying Kubernetes at scale"}
	input := &BulletRewriteInput{
		Ctx:                 context.Background(),
		LLM:                 stub,
		Log:                 slog.Default(),
		ResumeText:          resume,
		JDKeywords:          []string{"Kubernetes"},
		AccomplishmentsText: "Delivered Kubernetes migration on time",
		Defaults:            defaults,
		MaxRewrites:         5,
	}

	modified, changes, _, err := rewriteBullets(input)
	if err != nil {
		t.Fatalf("rewriteBullets returned error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected at least one BulletChange")
	}
	if !strings.Contains(modified, "Led a cross-functional team") {
		t.Errorf("modified resume does not contain rewritten bullet, got:\n%s", modified)
	}
}

func TestRewriteBullets_BulletMarkerPreserved(t *testing.T) {
	resume := `## Experience

• Improved deployment pipeline latency by 40% using Kubernetes

## Skills
Python
`
	defaults, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	defaults.Tailor.MaxTier2BulletRewrites = 5

	// LLM returns with "- " prefix — the original "•" prefix must be preserved.
	stub := &stubLLMClient{response: "- Reduced deployment latency 40% by automating Kubernetes rollouts"}
	input := &BulletRewriteInput{
		Ctx:                 context.Background(),
		LLM:                 stub,
		Log:                 slog.Default(),
		ResumeText:          resume,
		JDKeywords:          []string{"Kubernetes"},
		AccomplishmentsText: "Automated Kubernetes rollouts",
		Defaults:            defaults,
		MaxRewrites:         5,
	}

	modified, changes, _, err := rewriteBullets(input)
	if err != nil {
		t.Fatalf("rewriteBullets returned error: %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("expected at least one BulletChange")
	}

	// The rewritten line in the resume must use "•" not "-".
	for _, line := range strings.Split(modified, "\n") {
		if strings.Contains(line, "Reduced deployment latency") {
			if !strings.HasPrefix(strings.TrimSpace(line), "•") {
				t.Errorf("original bullet marker '•' not preserved; got line: %q", line)
			}
		}
	}
}
