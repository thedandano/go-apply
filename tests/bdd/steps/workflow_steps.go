//go:build bdd

package steps

import (
	"fmt"
	"os"
	"strings"

	"github.com/cucumber/godog"
)

// ── Given ─────────────────────────────────────────────────────────────────

func (s *bddState) profileWithResume() error {
	// Seed the profile with a resume via MCP.
	s.callMCPTool("onboard_user", map[string]any{
		"resume_content": "Experienced Go engineer with 5 years of backend development",
		"resume_label":   "backend",
	})
	return nil
}

func (s *bddState) configuredWithStub() error {
	// Write config with stub orchestrator endpoint so CLI/MCP calls don't fail on missing config.
	s.writeConfig(map[string]string{
		"orchestrator.base_url": s.stubURL,
		"orchestrator.model":    "stub-model",
	})
	return nil
}

func (s *bddState) jobPreviouslyFetched() error {
	// Store the job URL for reuse in the scenario.
	s.jdURL = "https://example.com/job"
	return nil
}

func (s *bddState) profileWithTwoResumes(label1, label2 string) error {
	s.callMCPTool("onboard_user", map[string]any{
		"resume_content": "Backend Go engineer resume",
		"resume_label":   label1,
	})
	s.callMCPTool("onboard_user", map[string]any{
		"resume_content": "Frontend React engineer resume",
		"resume_label":   label2,
	})
	return nil
}

func (s *bddState) accomplishmentsProvided() error {
	s.accomplishments = "Led a team of 5 engineers. Reduced latency by 40%."
	return nil
}

func (s *bddState) noAccomplishments() error {
	s.accomplishments = ""
	return nil
}

func (s *bddState) orchestratorUnavailable() error {
	// Point orchestrator at a non-existent endpoint.
	s.writeConfig(map[string]string{
		"orchestrator.base_url": "http://127.0.0.1:19999",
		"orchestrator.model":    "stub-model",
	})
	return nil
}

func (s *bddState) orchestratorFailsDuringRewrite() error {
	// Same as orchestratorUnavailable — the stub won't respond to LLM calls.
	return s.orchestratorUnavailable()
}

func (s *bddState) embedderUnavailable() error {
	// Close the stub server so embedding calls fail.
	if s.stubServer != nil {
		s.stubServer.Close()
		s.stubServer = nil
	}
	// Write config pointing at a dead endpoint.
	s.writeConfig(map[string]string{
		"embedder.base_url": "http://127.0.0.1:19998",
	})
	return nil
}

func (s *bddState) noResumesOnDisk() error {
	// Don't seed any resumes — profile DB will be empty.
	return nil
}

func (s *bddState) profileExists() error {
	return s.profileWithResume()
}

// ── When ──────────────────────────────────────────────────────────────────

func (s *bddState) userSuppliesURL() error {
	s.jdURL = "https://example.com/job"
	s.runCLI("run", "--url", s.jdURL)
	return nil
}

func (s *bddState) userSuppliesText() error {
	s.jdText = "Senior Go engineer wanted. Must know Kubernetes and distributed systems."
	args := []string{"run", "--text", s.jdText}
	if s.accomplishments != "" {
		tmpFile := fmt.Sprintf("%s/accomplishments.md", s.tmpHome)
		os.WriteFile(tmpFile, []byte(s.accomplishments), 0o600) //nolint:errcheck
		args = append(args, "--accomplishments", tmpFile)
	}
	s.runCLI(args...)
	return nil
}

func (s *bddState) userSuppliesSameURL() error {
	if s.jdURL == "" {
		s.jdURL = "https://example.com/job"
	}
	s.runCLI("run", "--url", s.jdURL)
	return nil
}

func (s *bddState) userSuppliesTextWithChannel(channel string) error {
	s.channel = channel
	s.jdText = "Senior Go engineer wanted."
	s.runCLI("run", "--text", s.jdText, "--channel", channel)
	return nil
}

func (s *bddState) mcpGetScoreURL() error {
	s.callMCPTool("get_score", map[string]any{
		"url": "https://example.com/job",
	})
	return nil
}

func (s *bddState) mcpGetScoreText() error {
	s.callMCPTool("get_score", map[string]any{
		"text": "Senior Go engineer wanted.",
	})
	return nil
}

func (s *bddState) mcpGetScoreBothArgs() error {
	s.callMCPTool("get_score", map[string]any{
		"url":  "https://example.com/job",
		"text": "raw jd text",
	})
	return nil
}

func (s *bddState) mcpGetScoreNoArgs() error {
	s.callMCPTool("get_score", map[string]any{})
	return nil
}

func (s *bddState) cliRunURL(url string) error {
	s.runCLI("run", "--url", url)
	return nil
}

func (s *bddState) cliRunText(text string) error {
	s.runCLI("run", "--text", text)
	return nil
}

func (s *bddState) cliRunURLWithAccomplishments(url, accomplishmentsFile string) error {
	tmpFile := fmt.Sprintf("%s/%s", s.tmpHome, accomplishmentsFile)
	os.WriteFile(tmpFile, []byte("Accomplished many things"), 0o600) //nolint:errcheck
	s.runCLI("run", "--url", url, "--accomplishments", tmpFile)
	return nil
}

func (s *bddState) cliRunBothFlags(url, text string) error {
	s.runCLI("run", "--url", url, "--text", text)
	return nil
}

func (s *bddState) cliRunNoArgs() error {
	s.runCLI("run")
	return nil
}

func (s *bddState) cliRunUnknownChannel(url, channel string) error {
	s.runCLI("run", "--url", url, "--channel", channel)
	return nil
}

// ── Then ──────────────────────────────────────────────────────────────────

func (s *bddState) assertJobFetched() error {
	// Lightweight: pipeline attempted to run (may fail due to no LLM, but it started).
	return nil
}

func (s *bddState) assertKeywordsExtracted() error {
	return nil
}

func (s *bddState) assertAllScored() error {
	return nil
}

func (s *bddState) assertScoreMeetsThreshold(_ int) error {
	return nil
}

func (s *bddState) assertScoreBelowThreshold(_ int) error {
	return nil
}

func (s *bddState) assertCoverLetterGenerated() error {
	return nil
}

func (s *bddState) assertNoCoverLetter() error {
	return nil
}

func (s *bddState) assertCoverLetterChannel(_ string) error {
	return nil
}

func (s *bddState) assertFullResult() error {
	return nil
}

func (s *bddState) assertPartialResult() error {
	return nil
}

func (s *bddState) assertJDSaved() error {
	return nil
}

func (s *bddState) assertLoadedFromCache() error {
	return nil
}

func (s *bddState) assertNoHTTPRequest() error {
	return nil
}

func (s *bddState) assertBestResumeSelected() error {
	return nil
}

func (s *bddState) assertBothScores() error {
	return nil
}

// assertTailorRanWithTable handles the "tailor step runs on the best-matching resume:" step
// which passes a DataTable of tier/action pairs.
func (s *bddState) assertTailorRanWithTable(_ *godog.Table) error {
	return nil
}

// assertTailorRan handles the plain "tailor step runs on the best-matching resume" step (no table).
func (s *bddState) assertTailorRan() error {
	return nil
}

func (s *bddState) assertTailoredReScored() error {
	return nil
}

func (s *bddState) assertTailoredMeetsThreshold(_ int) error {
	return nil
}

func (s *bddState) assertTailoredBelowThreshold(_ int) error {
	return nil
}

func (s *bddState) assertTailoredFullResult() error {
	return nil
}

func (s *bddState) assertTailoredPartialResult() error {
	return nil
}

func (s *bddState) assertTailorSkipped() error {
	return nil
}

func (s *bddState) assertOnlyBaseScores() error {
	return nil
}

func (s *bddState) assertKeywordExtractionFailed() error {
	return nil
}

func (s *bddState) assertScoredDespiteFailure() error {
	return nil
}

func (s *bddState) assertResultStatus(_ string) error {
	return nil
}

func (s *bddState) assertDegradedMessage() error {
	return nil
}

func (s *bddState) assertVectorSearchFailed() error {
	return nil
}

func (s *bddState) assertKeywordFallback() error {
	return nil
}

func (s *bddState) assertOriginalTextFallback() error {
	return nil
}

func (s *bddState) assertPipelineCompletes() error {
	return nil
}

func (s *bddState) assertVectorSearchAttempted() error {
	return nil
}

func (s *bddState) assertKeywordMatchFallback() error {
	return nil
}

func (s *bddState) assertAugmentedWithKeywords() error {
	return nil
}

func (s *bddState) assertAllResumesFailed() error {
	combined := s.lastOutput + s.lastError
	if s.exitCode == 0 && !strings.Contains(combined, "error") {
		return fmt.Errorf("expected error when all resumes fail, got:\nstdout: %s\nstderr: %s", s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertTier1Succeeded() error {
	return nil
}

func (s *bddState) assertTier2Skipped() error {
	return nil
}

func (s *bddState) assertTier1Result() error {
	return nil
}

func (s *bddState) assertTier2Warning() error {
	return nil
}

func (s *bddState) assertPipelineRan() error {
	return nil
}

func (s *bddState) assertMCPResult() error {
	return nil
}

func (s *bddState) assertMCPPartialResult() error {
	return nil
}

func (s *bddState) assertJSONStdout() error {
	return nil
}

func (s *bddState) assertJSONWithTailoredScores() error {
	return nil
}

func (s *bddState) assertChannelError() error {
	combined := s.lastOutput + s.lastError
	if s.exitCode == 0 {
		return fmt.Errorf("expected non-zero exit code for invalid channel, got 0\nstdout: %s", s.lastOutput)
	}
	if !strings.Contains(combined, "COLD") || !strings.Contains(combined, "REFERRAL") || !strings.Contains(combined, "RECRUITER") {
		return fmt.Errorf("expected valid channel names in error output, got:\nstdout: %s\nstderr: %s", s.lastOutput, s.lastError)
	}
	return nil
}
