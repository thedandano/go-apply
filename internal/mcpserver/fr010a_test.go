package mcpserver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/mcpserver"
	"github.com/thedandano/go-apply/internal/model"
)

// TestCheckFR010a tests the exported CheckFR010a wrapper.
// We exercise the logic through handleSubmitTailorT1 indirectly via the exported test helper.

// writeFR010aProfile writes a compiled profile with the given evidenced skills.
func writeFR010aProfile(t *testing.T, dir string, evidencedSkills []string) {
	t.Helper()
	var stories []model.Story
	if len(evidencedSkills) > 0 {
		stories = append(stories, model.Story{
			ID:         "story-001",
			SourceFile: "onboard",
			Text:       "story text",
			Skills:     evidencedSkills,
			Format:     "SBI",
			Type:       model.StoryTypeTechnical,
			JobTitle:   "Engineer",
		})
	}
	profile := model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now().UTC(),
		Stories:       stories,
	}
	data, _ := json.Marshal(profile)
	if err := os.WriteFile(filepath.Join(dir, "profile-compiled.json"), data, 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}

// TestCheckFR010a_AllEvidenced verifies nil is returned when all edit tokens are evidenced.
func TestCheckFR010a_AllEvidenced(t *testing.T) {
	dir := t.TempDir()
	writeFR010aProfile(t, dir, []string{"Go", "Kubernetes"})

	env := mcpserver.CheckFR010aForTest(context.Background(), "ses1", dir, "Go, Kubernetes")
	if env != nil {
		t.Errorf("expected nil (all evidenced), got: %+v", env)
	}
}

// TestCheckFR010a_UnevidencedToken verifies an error envelope is returned for unknown skill tokens.
func TestCheckFR010a_UnevidencedToken(t *testing.T) {
	dir := t.TempDir()
	writeFR010aProfile(t, dir, []string{"Go"})

	env := mcpserver.CheckFR010aForTest(context.Background(), "ses1", dir, "Go, Terraform")
	if env == nil {
		t.Fatal("expected error envelope for unevidenced token, got nil")
	}
	if env.Error == nil {
		t.Fatal("envelope.error is nil")
	}
	if env.Error.Code != "unevidenced_skill" {
		t.Errorf("code=%q; want unevidenced_skill", env.Error.Code)
	}
	details, ok := env.Error.Details.(map[string]interface{})
	if !ok {
		t.Fatalf("details is %T, want map", env.Error.Details)
	}
	tokens, _ := details["unevidenced_tokens"].([]string)
	if len(tokens) == 0 {
		t.Error("unevidenced_tokens empty in details")
	}
}

// TestCheckFR010a_ProfileMissing verifies nil is returned and the skip is logged
// when no compiled profile exists (new user who hasn't run compile_profile).
func TestCheckFR010a_ProfileMissing(t *testing.T) {
	dir := t.TempDir() // no profile-compiled.json

	env := mcpserver.CheckFR010aForTest(context.Background(), "ses1", dir, "Go")
	if env != nil {
		t.Errorf("expected nil when profile missing, got: %+v", env)
	}
}

// TestCheckFR010a_ReplaceOp verifies that EditOpReplace edits are also checked.
// A regression that only checked EditOpAdd would silently allow unevidenced replace edits.
func TestCheckFR010a_ReplaceOp(t *testing.T) {
	dir := t.TempDir()
	writeFR010aProfile(t, dir, []string{"Go"})

	env := mcpserver.CheckFR010aReplaceForTest(context.Background(), "ses1", dir, "Go, Terraform")
	if env == nil {
		t.Fatal("expected error envelope for unevidenced token via replace op, got nil")
	}
	if env.Error.Code != "unevidenced_skill" {
		t.Errorf("code=%q; want unevidenced_skill", env.Error.Code)
	}
	details, _ := env.Error.Details.(map[string]interface{})
	tokens, _ := details["unevidenced_tokens"].([]string)
	if len(tokens) == 0 || tokens[0] != "Terraform" {
		t.Errorf("unevidenced_tokens=%v; want [Terraform]", tokens)
	}
}

// TestCheckFR010a_MultipleUnevidenced verifies all unevidenced tokens appear in the details.
func TestCheckFR010a_MultipleUnevidenced(t *testing.T) {
	dir := t.TempDir()
	writeFR010aProfile(t, dir, []string{"Go"})

	env := mcpserver.CheckFR010aForTest(context.Background(), "ses1", dir, "Terraform, ArgoCD, Go")
	if env == nil {
		t.Fatal("expected error envelope")
	}
	details, _ := env.Error.Details.(map[string]interface{})
	tokens, _ := details["unevidenced_tokens"].([]string)
	if len(tokens) != 2 {
		t.Errorf("unevidenced_tokens len=%d; want 2 (Terraform, ArgoCD); tokens=%v", len(tokens), tokens)
	}
}
