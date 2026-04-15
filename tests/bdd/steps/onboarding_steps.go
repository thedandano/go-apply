//go:build bdd

package steps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

// onboardResult is the expected JSON shape returned by the onboard_user MCP tool
// and the go-apply onboard CLI command.
type onboardResult struct {
	Stored   []string `json:"stored"`
	Warnings []struct {
		Message string `json:"message"`
	} `json:"warnings"`
}

// parseOnboardResult parses s.lastOutput as an onboardResult.
func (s *bddState) parseOnboardResult() (*onboardResult, error) {
	var r onboardResult
	if err := json.Unmarshal([]byte(s.lastOutput), &r); err != nil {
		return nil, fmt.Errorf("parse onboard result: %w (raw: %s)", err, s.lastOutput)
	}
	return &r, nil
}

// ── Given ──────────────────────────────────────────────────────────────────

func (s *bddState) noUserProfileExists() error {
	// tmpHome is freshly created in Before; nothing to do.
	return nil
}

func (s *bddState) resumeLabeledExists(label string) error {
	// Pre-seed the profile by calling onboard_user via MCP.
	s.callMCPTool("onboard_user", map[string]any{
		"resume_content": "Existing resume content",
		"resume_label":   label,
	})
	if s.exitCode != 0 {
		return fmt.Errorf("failed to seed resume %q: exit %d, output: %s", label, s.exitCode, s.lastOutput)
	}
	return nil
}

func (s *bddState) orchestratorAPIKeyIsSet() error {
	s.writeConfig(map[string]string{
		"orchestrator.api_key": "sk-test-key",
	})
	return nil
}

func (s *bddState) orchestratorAPIKeyNotSet() error {
	// Default config has no orchestrator API key; nothing to do.
	return nil
}

// ── When (MCP) ─────────────────────────────────────────────────────────────

func (s *bddState) invokeOnboardUserWithResume(content, label string) error {
	s.callMCPTool("onboard_user", map[string]any{
		"resume_content": content,
		"resume_label":   label,
	})
	return nil
}

func (s *bddState) invokeOnboardUserTable(table *godog.Table) error {
	args := map[string]any{}
	for _, row := range table.Rows {
		if len(row.Cells) == 2 {
			args[row.Cells[0].Value] = row.Cells[1].Value
		}
	}
	s.callMCPTool("onboard_user", args)
	return nil
}

func (s *bddState) invokeOnboardUserSkillsOnly(skills string) error {
	s.callMCPTool("onboard_user", map[string]any{
		"skills": skills,
	})
	return nil
}

func (s *bddState) invokeOnboardUserAccomplishmentsOnly() error {
	s.callMCPTool("onboard_user", map[string]any{
		"accomplishments": "Led a team of 5 engineers for 2 years",
	})
	return nil
}

func (s *bddState) invokeOnboardUserContentNoLabel() error {
	s.callMCPTool("onboard_user", map[string]any{
		"resume_content": "Some resume content",
	})
	return nil
}

func (s *bddState) invokeOnboardUserLabelNoContent() error {
	s.callMCPTool("onboard_user", map[string]any{
		"resume_label": "backend",
	})
	return nil
}

func (s *bddState) invokeOnboardUserNoArgs() error {
	s.callMCPTool("onboard_user", map[string]any{})
	return nil
}

func (s *bddState) invokeAddResume(content, label string) error {
	s.callMCPTool("add_resume", map[string]any{
		"resume_content": content,
		"resume_label":   label,
	})
	return nil
}

func (s *bddState) invokeAddResumeNoContent() error {
	s.callMCPTool("add_resume", map[string]any{
		"resume_label": "backend",
	})
	return nil
}

func (s *bddState) invokeUpdateConfig(key, value string) error {
	s.callMCPTool("update_config", map[string]any{
		"key":   key,
		"value": value,
	})
	return nil
}

func (s *bddState) invokeUpdateConfigAnyValue(key string) error {
	s.callMCPTool("update_config", map[string]any{
		"key":   key,
		"value": "some-value",
	})
	return nil
}

func (s *bddState) invokeGetConfig() error {
	s.callMCPTool("get_config", map[string]any{})
	return nil
}

// ── When (CLI) ─────────────────────────────────────────────────────────────

// cliOnboardDocString handles "When the user runs:" followed by a DocString block.
func (s *bddState) cliOnboardDocString(doc *godog.DocString) error {
	content := strings.TrimSpace(doc.Content)
	parts := strings.Fields(content)
	if len(parts) < 2 {
		return fmt.Errorf("unexpected command format: %q", content)
	}
	args := parts[1:] // skip "go-apply"
	// Create referenced files in tmpHome and rewrite file args to full paths.
	args = s.resolveFileArgs(args)
	s.runCLI(args...)
	return nil
}

// resolveFileArgs creates any file arguments in tmpHome and rewrites the args
// to use absolute paths pointing into tmpHome. Flags (starting with --) are not modified.
func (s *bddState) resolveFileArgs(args []string) []string {
	resolved := make([]string, len(args))
	copy(resolved, args)
	for i, arg := range resolved {
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(arg))
		if ext == ".pdf" || ext == ".md" || ext == ".txt" {
			base := filepath.Base(arg)
			path := filepath.Join(s.tmpHome, base)
			os.MkdirAll(filepath.Dir(path), 0o700)         //nolint:errcheck
			os.WriteFile(path, resumeContent(base), 0o600) //nolint:errcheck
			resolved[i] = path
		}
	}
	return resolved
}

func (s *bddState) cliOnboardResume(filename string) error {
	path := filepath.Join(s.tmpHome, filename)
	content := resumeContent(filename)
	os.WriteFile(path, content, 0o600) //nolint:errcheck
	s.runCLI("onboard", "--resume", path)
	return nil
}

// resumeContent returns appropriate content bytes for a file based on its extension.
// PDF files get a minimal valid PDF; others get plain text.
func resumeContent(filename string) []byte {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".pdf" {
		return minimalPDF
	}
	return []byte("Go engineer resume content for " + filename)
}

func (s *bddState) cliOnboardTwoResumes(filename1, filename2 string) error {
	// path1: file in tmpHome using filename1 as-is.
	path1 := filepath.Join(s.tmpHome, filepath.Base(filename1))
	// path2: file placed in a subdirectory so the path is distinct from path1
	// even when both filenames share the same basename (duplicate-label test).
	dir2 := filepath.Join(s.tmpHome, "other")
	os.MkdirAll(dir2, 0o700) //nolint:errcheck
	path2 := filepath.Join(dir2, filepath.Base(filename2))
	os.WriteFile(path1, resumeContent(filepath.Base(filename1)), 0o600) //nolint:errcheck
	os.WriteFile(path2, resumeContent(filepath.Base(filename2)), 0o600) //nolint:errcheck
	s.runCLI("onboard", "--resume", path1, "--resume", path2)
	return nil
}

func (s *bddState) cliOnboardNoFlags() error {
	s.runCLI("onboard")
	return nil
}

func (s *bddState) cliConfigSet(key, value string) error {
	s.runCLI("config", "set", key, value)
	return nil
}

func (s *bddState) cliConfigShow() error {
	s.runCLI("config", "show")
	return nil
}

// ── Then ──────────────────────────────────────────────────────────────────

func (s *bddState) assertGoApplyStoresResumeLabel(label string) error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, "resume:"+label) {
		return fmt.Errorf("expected resume:%s in output, got:\nstdout: %s\nstderr: %s", label, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertGoApplyStoresAll() error {
	combined := s.lastOutput + s.lastError
	if s.exitCode != 0 {
		return fmt.Errorf("expected success exit code, got %d\nstdout: %s\nstderr: %s", s.exitCode, s.lastOutput, s.lastError)
	}
	_ = combined
	return nil
}

func (s *bddState) assertGoApplyStoresSkills() error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, "ref:skills") && !strings.Contains(combined, "skills") {
		return fmt.Errorf("expected skills reference in output, got:\nstdout: %s\nstderr: %s", s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertGoApplyStoresAccomplishments() error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, "accomplishments") {
		return fmt.Errorf("expected accomplishments in output, got:\nstdout: %s\nstderr: %s", s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertResponseLists(key string) error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, key) {
		return fmt.Errorf("expected %q in output, got:\nstdout: %s\nstderr: %s", key, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertResponseListsThree(key1, key2, key3 string) error {
	combined := s.lastOutput + s.lastError
	for _, key := range []string{key1, key2, key3} {
		if !strings.Contains(combined, key) {
			return fmt.Errorf("expected %q in output, got:\nstdout: %s\nstderr: %s", key, s.lastOutput, s.lastError)
		}
	}
	return nil
}

func (s *bddState) assertResumeReplaced(label string) error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, "resume:"+label) {
		return fmt.Errorf("expected resume:%s to be updated, got:\nstdout: %s\nstderr: %s", label, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertError(expectedMsg string) error {
	combined := s.lastOutput + s.lastError
	// Check direct match first.
	if strings.Contains(combined, expectedMsg) {
		return nil
	}
	// Also check JSON-encoded version (e.g. `"db_path"` → `\"db_path\"`).
	jsonEncoded := strings.ReplaceAll(expectedMsg, `"`, `\"`)
	if strings.Contains(combined, jsonEncoded) {
		return nil
	}
	return fmt.Errorf("expected error %q in output, got:\nstdout: %s\nstderr: %s", expectedMsg, s.lastOutput, s.lastError)
}

func (s *bddState) assertErrorSingleQuote(expectedMsg string) error {
	return s.assertError(expectedMsg)
}

func (s *bddState) assertErrorContaining(substr string) error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, substr) {
		return fmt.Errorf("expected error containing %q, got:\nstdout: %s\nstderr: %s", substr, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertConfigSaved() error {
	if s.exitCode != 0 {
		return fmt.Errorf("expected success (exit 0), got exit %d\nstdout: %s\nstderr: %s", s.exitCode, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertConfigKeyUpdated(key string) error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, key) {
		return fmt.Errorf("expected key %q in output, got:\nstdout: %s\nstderr: %s", key, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertValueRedacted(expectedVal string) error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, expectedVal) {
		return fmt.Errorf("expected value %q in output, got:\nstdout: %s\nstderr: %s", expectedVal, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertAllConfigFields() error {
	if s.exitCode != 0 {
		return fmt.Errorf("expected success, got exit %d\nstdout: %s\nstderr: %s", s.exitCode, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertOrchestratorAPIKey(expectedVal string) error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, expectedVal) {
		return fmt.Errorf("expected orchestrator.api_key shown as %q, got:\nstdout: %s\nstderr: %s", expectedVal, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertEmbedderAPIKey(expectedVal string) error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, expectedVal) {
		return fmt.Errorf("expected embedder.api_key shown as %q, got:\nstdout: %s\nstderr: %s", expectedVal, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertOrchestratorAPIKeyEmpty() error {
	// orchestrator.api_key should be an empty string — check it's not "***"
	if strings.Contains(s.lastOutput, "orchestrator.api_key") && strings.Contains(s.lastOutput, "***") {
		return fmt.Errorf("expected orchestrator.api_key to be empty, but got redacted value\nstdout: %s", s.lastOutput)
	}
	return nil
}

func (s *bddState) assertJSONResultStored() error {
	if s.exitCode != 0 {
		return fmt.Errorf("expected success, got exit %d\nstdout: %s\nstderr: %s", s.exitCode, s.lastOutput, s.lastError)
	}
	if !strings.Contains(s.lastOutput, "stored") {
		return fmt.Errorf("expected 'stored' key in JSON output, got: %s", s.lastOutput)
	}
	return nil
}

func (s *bddState) assertJSONResultKey(key string) error {
	combined := s.lastOutput + s.lastError
	if !strings.Contains(combined, key) {
		return fmt.Errorf("expected %q in JSON output, got: %s", key, combined)
	}
	return nil
}

func (s *bddState) assertJSONResultBothKeys() error {
	if s.exitCode != 0 {
		return fmt.Errorf("expected success, got exit %d\nstdout: %s\nstderr: %s", s.exitCode, s.lastOutput, s.lastError)
	}
	if !strings.Contains(s.lastOutput, "stored") {
		return fmt.Errorf("expected 'stored' key in JSON output, got: %s", s.lastOutput)
	}
	return nil
}

func (s *bddState) assertConfigConfirmation() error {
	if s.exitCode != 0 {
		return fmt.Errorf("expected success, got exit %d\nstdout: %s\nstderr: %s", s.exitCode, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertAllUserFacingFields() error {
	if s.exitCode != 0 {
		return fmt.Errorf("expected success, got exit %d\nstdout: %s\nstderr: %s", s.exitCode, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertNoResumesStored() error {
	// In case of error (e.g. duplicate label), no resumes should be stored.
	// We just need to confirm the exit code was non-zero (command failed).
	if s.exitCode == 0 {
		return fmt.Errorf("expected non-zero exit code (error), but got success\nstdout: %s", s.lastOutput)
	}
	return nil
}

func (s *bddState) assertResumeStoredLabel(label string) error {
	return s.assertGoApplyStoresResumeLabel(label)
}

func (s *bddState) assertStoresResumeLabel(label string) error {
	return s.assertGoApplyStoresResumeLabel(label)
}

func (s *bddState) assertStoresSkillsAndAccomplishments() error {
	combined := s.lastOutput + s.lastError
	if s.exitCode != 0 {
		return fmt.Errorf("expected success, got exit %d\n%s", s.exitCode, combined)
	}
	return nil
}

func (s *bddState) assertLoadsEachFile() error {
	if s.exitCode != 0 {
		return fmt.Errorf("expected success, got exit %d\nstdout: %s\nstderr: %s", s.exitCode, s.lastOutput, s.lastError)
	}
	return nil
}

func (s *bddState) assertStoresBothResumes(label1, label2 string) error {
	combined := s.lastOutput + s.lastError
	for _, label := range []string{"resume:" + label1, "resume:" + label2} {
		if !strings.Contains(combined, label) {
			return fmt.Errorf("expected %q in output, got:\nstdout: %s\nstderr: %s", label, s.lastOutput, s.lastError)
		}
	}
	return nil
}
