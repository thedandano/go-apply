package tailor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/logger"
	"github.com/thedandano/go-apply/internal/model"
)

func defaultsForTest(t *testing.T) *config.AppDefaults {
	t.Helper()
	d, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults: %v", err)
	}
	return d
}

func newTestService(llm *stubLLMClient, defaults *config.AppDefaults) *Service {
	return New(llm, defaults, slog.Default())
}

func TestTailorResume_Tier1Only(t *testing.T) {
	defaults := defaultsForTest(t)
	defaults.Tailor.MaxTier2BulletRewrites = 0 // tier-2 disabled via options

	resume := `## Skills
Python

## Experience

- Built distributed systems
`
	input := &model.TailorInput{
		Resume:     model.ResumeFile{Label: "main"},
		ResumeText: resume,
		JD: model.JDData{
			Required:  []string{"Golang"},
			Preferred: []string{"Kubernetes"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 0},
	}

	svc := newTestService(&stubLLMClient{response: "no-op"}, defaults)
	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("TailorResume: %v", err)
	}

	if result.TierApplied != model.TierKeyword {
		t.Errorf("tier = %v, want TierKeyword", result.TierApplied)
	}
	if len(result.RewrittenBullets) != 0 {
		t.Errorf("expected no rewritten bullets for tier-1 only, got %d", len(result.RewrittenBullets))
	}
	if result.TailoredText == "" {
		t.Error("TailoredText must not be empty")
	}
	if !strings.Contains(result.TailoredText, "Golang") {
		t.Error("TailoredText should contain the injected keyword Golang")
	}
	if result.ResumeLabel != "main" {
		t.Errorf("ResumeLabel = %q, want 'main'", result.ResumeLabel)
	}
}

func TestTailorResume_Tier2LLMError_Degrades(t *testing.T) {
	defaults := defaultsForTest(t)
	defaults.Tailor.MaxTier2BulletRewrites = 5

	resume := `## Skills
Python

## Experience

- Deployed Kubernetes clusters at scale
`
	input := &model.TailorInput{
		Resume:              model.ResumeFile{Label: "main"},
		ResumeText:          resume,
		AccomplishmentsText: "Managed large Kubernetes deployments",
		JD: model.JDData{
			Required: []string{"Kubernetes"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 5},
	}

	// LLM always errors on every bullet call.
	// rewriteBullets swallows per-bullet errors and skips them, producing zero
	// changes. TailorResume then keeps the tier-1 result (TierKeyword) unchanged.
	// This is the defined internal-degradation path — no error is surfaced.
	svc := newTestService(&stubLLMClient{err: errors.New("llm unavailable")}, defaults)
	result, err := svc.TailorResume(context.Background(), input)

	// Must degrade gracefully — no error returned.
	if err != nil {
		t.Fatalf("expected no error on tier-2 LLM failure, got: %v", err)
	}
	// TierApplied must remain TierKeyword: no bullet changes were produced.
	if result.TierApplied != model.TierKeyword {
		t.Errorf("tier = %v, want TierKeyword when all tier-2 bullet rewrites fail", result.TierApplied)
	}
	// TailoredText must be the tier-1 output (non-empty) containing the injected keyword.
	if result.TailoredText == "" {
		t.Error("TailoredText must not be empty even when tier-2 degrades")
	}
	// Tier-1 should have injected "Kubernetes" into the Skills section.
	if !strings.Contains(result.TailoredText, "Kubernetes") {
		t.Errorf("TailoredText should contain tier-1 injected keyword 'Kubernetes'; got:\n%s", result.TailoredText)
	}
	// No rewritten bullets: tier-2 produced zero changes.
	if len(result.RewrittenBullets) != 0 {
		t.Errorf("expected no rewritten bullets when LLM fails, got %d", len(result.RewrittenBullets))
	}
}

// TestTailorResume_Tier1Text_NonEmpty asserts that Tier1Text is set after T1 runs (M5.4a).
func TestTailorResume_Tier1Text_NonEmpty(t *testing.T) {
	defaults := defaultsForTest(t)
	defaults.Tailor.MaxTier2BulletRewrites = 0 // T2 disabled

	input := &model.TailorInput{
		Resume:     model.ResumeFile{Label: "main"},
		ResumeText: "## Skills\nPython\n\n## Experience\n\n- Built systems\n",
		JD: model.JDData{
			Required: []string{"Golang"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 0},
	}

	svc := newTestService(&stubLLMClient{response: "no-op"}, defaults)
	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("TailorResume: %v", err)
	}
	if result.Tier1Text == "" {
		t.Error("Tier1Text must be non-empty when T1 runs")
	}
	if !strings.Contains(result.Tier1Text, "Golang") {
		t.Errorf("Tier1Text should contain the injected keyword 'Golang'; got:\n%s", result.Tier1Text)
	}
}

// TestTailorResume_Tier1Score_NilFromTailor asserts that Tier1Score is nil when set
// by the tailor service alone — only the pipeline sets it (M5.4b).
func TestTailorResume_Tier1Score_NilFromTailor(t *testing.T) {
	defaults := defaultsForTest(t)
	defaults.Tailor.MaxTier2BulletRewrites = 0

	input := &model.TailorInput{
		Resume:     model.ResumeFile{Label: "main"},
		ResumeText: "## Skills\nPython\n\n## Experience\n\n- Built systems\n",
		JD: model.JDData{
			Required: []string{"Golang"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 0},
	}

	svc := newTestService(&stubLLMClient{response: "no-op"}, defaults)
	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("TailorResume: %v", err)
	}
	if result.Tier1Score != nil {
		t.Error("Tier1Score must be nil when returned from the tailor service directly — only the pipeline sets it")
	}
}

func TestTailorResume_Tier2RewritesBullets(t *testing.T) {
	defaults := defaultsForTest(t)
	defaults.Tailor.MaxTier2BulletRewrites = 5

	resume := `## Skills
Python

## Experience

- Led Kubernetes migration for 50-node cluster
`
	input := &model.TailorInput{
		Resume:              model.ResumeFile{Label: "main"},
		ResumeText:          resume,
		AccomplishmentsText: "Completed Kubernetes migration 2 weeks ahead of schedule",
		JD: model.JDData{
			Required: []string{"Kubernetes"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 5},
	}

	stub := &stubLLMClient{response: "- Led Kubernetes migration of 50-node cluster 2 weeks ahead of schedule"}
	svc := newTestService(stub, defaults)
	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("TailorResume: %v", err)
	}

	if result.TierApplied != model.TierBullet {
		t.Errorf("tier = %v, want TierBullet", result.TierApplied)
	}
	if len(result.RewrittenBullets) == 0 {
		t.Error("expected at least one rewritten bullet")
	}
	if result.TailoredText == "" {
		t.Error("TailoredText must be set")
	}
	// Verify TailoredText contains the rewritten content.
	if !strings.Contains(result.TailoredText, "ahead of schedule") {
		t.Errorf("TailoredText does not contain rewritten bullet content; got:\n%s", result.TailoredText)
	}
}

// findLogRecord scans JSON log lines for the first record matching the given message.
func findLogRecord(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("failed to parse log line %q: %v", line, err)
		}
		if rec["msg"] == msg {
			return rec
		}
	}
	return nil
}

// TestTailorResume_CascadeCompleteInfoLog asserts the info summary log line is
// emitted at the end of every cascade path with the correct attributes.
func TestTailorResume_CascadeCompleteInfoLog(t *testing.T) {
	defaults := defaultsForTest(t)

	resume := "## Skills\nPython\n\n## Experience\n\n- Built distributed systems\n"
	input := &model.TailorInput{
		Resume:     model.ResumeFile{Label: "main"},
		ResumeText: resume,
		JD: model.JDData{
			Required:  []string{"Golang"},
			Preferred: []string{"Kubernetes"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 0},
	}

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	log := slog.New(handler)
	svc := New(&stubLLMClient{response: "no-op"}, defaults, log)

	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("TailorResume: %v", err)
	}

	rec := findLogRecord(t, &buf, "tailor cascade complete")
	if rec == nil {
		t.Fatalf("expected 'tailor cascade complete' log record; got:\n%s", buf.String())
	}

	// tier_applied is logged as an int (TailorTier is int), decoded as float64 in JSON.
	tierApplied, ok := rec["tier_applied"].(float64)
	if !ok {
		t.Errorf("tier_applied attribute missing or not a number; record: %v", rec)
	} else if model.TailorTier(tierApplied) != result.TierApplied {
		t.Errorf("tier_applied = %v, want %v", model.TailorTier(tierApplied), result.TierApplied)
	}

	// tailored_text_bytes must be a number equal to len(result.TailoredText).
	bytesVal, ok := rec["tailored_text_bytes"].(float64)
	if !ok {
		t.Errorf("tailored_text_bytes attribute missing or not a number; record: %v", rec)
	} else if int(bytesVal) != len(result.TailoredText) {
		t.Errorf("tailored_text_bytes = %d, want %d", int(bytesVal), len(result.TailoredText))
	}

	// tailored_text_lines must be a number equal to lineCount(result.TailoredText).
	linesVal, ok := rec["tailored_text_lines"].(float64)
	if !ok {
		t.Errorf("tailored_text_lines attribute missing or not a number; record: %v", rec)
	} else if int(linesVal) != lineCount(result.TailoredText) {
		t.Errorf("tailored_text_lines = %d, want %d", int(linesVal), lineCount(result.TailoredText))
	}
}

// TestTailorResume_FinalTextDebugLog_VerboseOn asserts that, when verbose is on,
// the debug "tailor final text" log carries the full tailored text.
func TestTailorResume_FinalTextDebugLog_VerboseOn(t *testing.T) {
	logger.SetVerbose(true)
	t.Cleanup(func() { logger.SetVerbose(false) })

	defaults := defaultsForTest(t)

	resume := "## Skills\nPython\n\n## Experience\n\n- Built distributed systems\n"
	input := &model.TailorInput{
		Resume:     model.ResumeFile{Label: "main"},
		ResumeText: resume,
		JD: model.JDData{
			Required: []string{"Golang"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 0},
	}

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	log := slog.New(handler)
	svc := New(&stubLLMClient{response: "no-op"}, defaults, log)

	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("TailorResume: %v", err)
	}

	rec := findLogRecord(t, &buf, "tailor final text")
	if rec == nil {
		t.Fatalf("expected 'tailor final text' debug record; got:\n%s", buf.String())
	}

	tailoredText, ok := rec["tailored_text"].(string)
	if !ok {
		t.Errorf("tailored_text attribute missing or not a string; record: %v", rec)
	} else if tailoredText != result.TailoredText {
		t.Errorf("tailored_text mismatch: got %q, want %q", tailoredText, result.TailoredText)
	}
}

// TestTailorResume_FinalTextDebugLog_VerboseOff asserts that, when verbose is off,
// the debug "tailor final text" log carries a truncated/redacted value, not the full text.
func TestTailorResume_FinalTextDebugLog_VerboseOff(t *testing.T) {
	logger.SetVerbose(false)

	defaults := defaultsForTest(t)

	// Build a resume long enough to exceed the 2048-byte payload limit so truncation fires.
	longBullet := strings.Repeat("- Built distributed systems at global scale with many technologies\n", 40)
	resume := "## Skills\nPython\n\n## Experience\n\n" + longBullet
	input := &model.TailorInput{
		Resume:     model.ResumeFile{Label: "main"},
		ResumeText: resume,
		JD: model.JDData{
			Required: []string{"Golang"},
		},
		Options: model.TailorOptions{MaxTier2BulletRewrites: 0},
	}

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	log := slog.New(handler)
	svc := New(&stubLLMClient{response: "no-op"}, defaults, log)

	result, err := svc.TailorResume(context.Background(), input)
	if err != nil {
		t.Fatalf("TailorResume: %v", err)
	}

	rec := findLogRecord(t, &buf, "tailor final text")
	if rec == nil {
		t.Fatalf("expected 'tailor final text' debug record; got:\n%s", buf.String())
	}

	tailoredText, ok := rec["tailored_text"].(string)
	if !ok {
		t.Errorf("tailored_text attribute missing or not a string; record: %v", rec)
		return
	}

	// When verbose is off, PayloadAttr truncates long text and inserts "bytes omitted".
	if len(result.TailoredText) > 2048 {
		if !strings.Contains(tailoredText, "bytes omitted") {
			t.Errorf("expected truncated payload with 'bytes omitted' marker when verbose=false; got %q", tailoredText)
		}
		if tailoredText == result.TailoredText {
			t.Errorf("expected tailored_text to be truncated when verbose=false, but got full text")
		}
	}
}
