package tailorllm_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/tailorllm"
)

// compile-time check: fakeStore satisfies tailorllm.TailorStore.
var _ tailorllm.TailorStore = (*fakeStore)(nil)

type fakeStore struct {
	openFunc func(bundle string, input *model.TailorInput, timeout time.Duration) (string, error)
	waitFunc func(ctx context.Context, id string) (model.TailorResult, error)
}

func (f *fakeStore) Open(bundle string, input *model.TailorInput, timeout time.Duration) (string, error) {
	return f.openFunc(bundle, input, timeout)
}

func (f *fakeStore) Wait(ctx context.Context, id string) (model.TailorResult, error) {
	return f.waitFunc(ctx, id)
}

// TestLLMTailor_HappyPath verifies that LLMTailor delegates to Open then Wait,
// threads the session ID through, and returns the store result unchanged.
// It also asserts that the bundle passed to Open contains both the configured
// PromptBody ("skill-text") and the resume text ("my resume").
func TestLLMTailor_HappyPath(t *testing.T) {
	const sessionID = "test-session-1"
	wantResult := model.TailorResult{
		TailoredText: "tailored output",
		Changelog: []model.ChangelogEntry{
			{Kind: model.ChangelogSkillAdd, Tier: model.ChangelogTier1},
		},
	}

	var capturedBundle string
	store := &fakeStore{
		openFunc: func(bundle string, _ *model.TailorInput, _ time.Duration) (string, error) {
			capturedBundle = bundle
			return sessionID, nil
		},
		waitFunc: func(_ context.Context, id string) (model.TailorResult, error) {
			if id != sessionID {
				t.Errorf("Wait called with id %q, want %q", id, sessionID)
			}
			return wantResult, nil
		},
	}

	tailor := tailorllm.New(tailorllm.Config{
		Timeout:    5 * time.Second,
		PromptBody: "skill-text",
	}, store)

	input := &model.TailorInput{ResumeText: "my resume"}
	result, err := tailor.TailorResume(context.Background(), input)

	if err != nil {
		t.Fatalf("TailorResume: unexpected error: %v", err)
	}
	if result.TailoredText != wantResult.TailoredText {
		t.Errorf("TailoredText = %q, want %q", result.TailoredText, wantResult.TailoredText)
	}
	if len(result.Changelog) != 1 {
		t.Errorf("len(Changelog) = %d, want 1", len(result.Changelog))
	}
	if !strings.Contains(capturedBundle, "skill-text") {
		t.Errorf("bundle does not contain PromptBody \"skill-text\"; got: %q", capturedBundle)
	}
	if !strings.Contains(capturedBundle, "my resume") {
		t.Errorf("bundle does not contain resume text \"my resume\"; got: %q", capturedBundle)
	}
}

// TestLLMTailor_Timeout verifies that when Wait returns ErrTailorSessionExpired,
// TailorResume surfaces an error that wraps the sentinel so callers can detect it.
func TestLLMTailor_Timeout(t *testing.T) {
	const sessionID = "test-session-2"

	store := &fakeStore{
		openFunc: func(_ string, _ *model.TailorInput, _ time.Duration) (string, error) {
			return sessionID, nil
		},
		waitFunc: func(_ context.Context, _ string) (model.TailorResult, error) {
			return model.TailorResult{}, mcpserver.ErrTailorSessionExpired
		},
	}

	tailor := tailorllm.New(tailorllm.Config{
		Timeout:    5 * time.Second,
		PromptBody: "skill-text",
	}, store)

	input := &model.TailorInput{ResumeText: "my resume"}
	_, err := tailor.TailorResume(context.Background(), input)

	if err == nil {
		t.Fatal("TailorResume: expected error, got nil")
	}
	if !errors.Is(err, mcpserver.ErrTailorSessionExpired) {
		t.Errorf("expected errors.Is(err, ErrTailorSessionExpired) true; err = %v", err)
	}
}
