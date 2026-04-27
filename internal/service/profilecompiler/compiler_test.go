package profilecompiler_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/profilecompiler"
	"github.com/thedandano/go-apply/internal/testutil/llmstub"
)

// buildPrompt returns the expected user-message key the compiler sends for a story.
// The compiler embeds the story text and skills list; we replicate the format here
// so tests can key the stub response correctly.
func storyPromptKey(storyText, skillsText string) string {
	return fmt.Sprintf("skills:\n%s\n\nstory:\n%s", skillsText, storyText)
}

const skillsText = "Go\nKubernetes\nTerraform"

// TestCompile_HappyPath — all stories tagged correctly.
func TestCompile_HappyPath(t *testing.T) {
	ctx := context.Background()
	story1 := model.RawStory{SourceFile: "accomplishments-0.md", Text: "story1 text"}
	story2 := model.RawStory{SourceFile: "accomplishments-1.md", Text: "story2 text"}

	stub := llmstub.New(map[string]string{
		storyPromptKey(story1.Text, skillsText): `["Go","Kubernetes"]`,
		storyPromptKey(story2.Text, skillsText): `["Terraform"]`,
	}, 0, "")

	compiler := profilecompiler.New(stub, nil)
	input := model.CompileInput{
		SkillsText: skillsText,
		Stories:    []model.RawStory{story1, story2},
	}
	profile, err := compiler.Compile(ctx, input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if profile.PartialTaggingFailure {
		t.Error("PartialTaggingFailure=true; want false")
	}
	if len(profile.Stories) != 2 {
		t.Fatalf("stories count = %d; want 2", len(profile.Stories))
	}
	assertSkills(t, profile.Stories[0], "Go", "Kubernetes")
	assertSkills(t, profile.Stories[1], "Terraform")
}

// TestCompile_ManyToMany — skill in multiple stories.
func TestCompile_ManyToMany(t *testing.T) {
	ctx := context.Background()
	story1 := model.RawStory{SourceFile: "accomplishments-0.md", Text: "s1"}
	story2 := model.RawStory{SourceFile: "accomplishments-1.md", Text: "s2"}
	story3 := model.RawStory{SourceFile: "accomplishments-2.md", Text: "s3"}

	stub := llmstub.New(map[string]string{
		storyPromptKey(story1.Text, skillsText): `["Go"]`,
		storyPromptKey(story2.Text, skillsText): `["Go","Kubernetes"]`,
		storyPromptKey(story3.Text, skillsText): `["Go","Terraform"]`,
	}, 0, "")

	compiler := profilecompiler.New(stub, nil)
	profile, err := compiler.Compile(ctx, model.CompileInput{
		SkillsText: skillsText,
		Stories:    []model.RawStory{story1, story2, story3},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// "Go" must appear in all three stories
	for i, s := range profile.Stories {
		if !containsSkill(s, "Go") {
			t.Errorf("story[%d] missing Go; skills=%v", i, s.Skills)
		}
	}
	if len(profile.OrphanedSkills) != 0 {
		t.Errorf("orphaned_skills=%v; want empty", profile.OrphanedSkills)
	}
}

// TestCompile_LLMFailure_SetsTaggingError.
func TestCompile_LLMFailure_SetsTaggingError(t *testing.T) {
	ctx := context.Background()
	story1 := model.RawStory{SourceFile: "accomplishments-0.md", Text: "s1"}
	story2 := model.RawStory{SourceFile: "accomplishments-1.md", Text: "s2"}

	// Fail on 1st LLM call (story1); story2 succeeds.
	stub := llmstub.New(map[string]string{
		storyPromptKey(story2.Text, skillsText): `["Kubernetes"]`,
	}, 1, "llm unavailable")

	compiler := profilecompiler.New(stub, nil)
	profile, err := compiler.Compile(ctx, model.CompileInput{
		SkillsText: skillsText,
		Stories:    []model.RawStory{story1, story2},
	})
	if err != nil {
		t.Fatalf("Compile returned hard error; want soft partial failure: %v", err)
	}
	if !profile.PartialTaggingFailure {
		t.Error("PartialTaggingFailure=false; want true")
	}
	if profile.Stories[0].TaggingError == "" {
		t.Error("story[0] TaggingError empty; want non-empty")
	}
	if profile.Stories[1].TaggingError != "" {
		t.Errorf("story[1] TaggingError=%q; want empty", profile.Stories[1].TaggingError)
	}
}

// TestCompile_FailedStoryExcludedFromOrphans.
func TestCompile_FailedStoryExcludedFromOrphans(t *testing.T) {
	ctx := context.Background()
	// skills: Go, Kubernetes, Terraform
	// story1 fails → its claimed skills don't count
	// story2 covers Go only
	// Kubernetes and Terraform must be orphaned; Go must NOT be orphaned.
	story1 := model.RawStory{SourceFile: "a.md", Text: "s1"} // will fail
	story2 := model.RawStory{SourceFile: "b.md", Text: "s2"}

	stub := llmstub.New(map[string]string{
		storyPromptKey(story2.Text, skillsText): `["Go"]`,
	}, 1, "timeout")

	compiler := profilecompiler.New(stub, nil)
	profile, err := compiler.Compile(ctx, model.CompileInput{
		SkillsText: skillsText,
		Stories:    []model.RawStory{story1, story2},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	orphanSkills := orphanSet(profile.OrphanedSkills)
	if orphanSkills["Go"] {
		t.Error("Go in orphaned_skills; it is covered by story2")
	}
	if !orphanSkills["Kubernetes"] {
		t.Error("Kubernetes not in orphaned_skills; story1 failed so it should be orphaned")
	}
	if !orphanSkills["Terraform"] {
		t.Error("Terraform not in orphaned_skills")
	}
}

// TestCompile_OrphanedSkills_UncoveredSkill — skill not in any story.
func TestCompile_OrphanedSkills_UncoveredSkill(t *testing.T) {
	ctx := context.Background()
	story := model.RawStory{SourceFile: "a.md", Text: "s"}
	stub := llmstub.New(map[string]string{
		storyPromptKey(story.Text, skillsText): `["Go","Kubernetes"]`,
	}, 0, "")

	compiler := profilecompiler.New(stub, nil)
	profile, err := compiler.Compile(ctx, model.CompileInput{
		SkillsText: skillsText,
		Stories:    []model.RawStory{story},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	orphans := orphanSet(profile.OrphanedSkills)
	if !orphans["Terraform"] {
		t.Error("Terraform not orphaned; want orphaned")
	}
}

// TestCompile_OrphanedSkills_PositiveConverse — covered skill never appears in orphans.
func TestCompile_OrphanedSkills_PositiveConverse(t *testing.T) {
	ctx := context.Background()
	// story1 fails, story2 covers Go. Go must NOT appear in orphaned_skills.
	story1 := model.RawStory{SourceFile: "a.md", Text: "fail"}
	story2 := model.RawStory{SourceFile: "b.md", Text: "good"}
	stub := llmstub.New(map[string]string{
		storyPromptKey(story2.Text, skillsText): `["Go"]`,
	}, 1, "err")

	compiler := profilecompiler.New(stub, nil)
	profile, err := compiler.Compile(ctx, model.CompileInput{
		SkillsText: skillsText,
		Stories:    []model.RawStory{story1, story2},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, o := range profile.OrphanedSkills {
		if o.Skill == "Go" {
			t.Error("Go appears in orphaned_skills despite being covered by story2")
		}
	}
}

// TestCompile_DeferredCarriesForward.
func TestCompile_DeferredCarriesForward(t *testing.T) {
	ctx := context.Background()
	stub := llmstub.New(map[string]string{}, 0, "")

	prior := &model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now(),
		OrphanedSkills: []model.OrphanedSkill{
			{Skill: "Kubernetes", Deferred: true},
		},
	}
	compiler := profilecompiler.New(stub, nil)
	profile, err := compiler.Compile(ctx, model.CompileInput{
		SkillsText:   "Go\nKubernetes",
		Stories:      []model.RawStory{{SourceFile: "a.md", Text: "about Go"}},
		PriorProfile: prior,
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	orphans := orphanSet(profile.OrphanedSkills)
	if !orphans["Kubernetes"] {
		t.Error("Kubernetes not orphaned; should still be orphaned since no story covers it")
	}
	for _, o := range profile.OrphanedSkills {
		if o.Skill == "Kubernetes" && !o.Deferred {
			t.Error("Kubernetes Deferred=false; want true (carried from prior)")
		}
	}
}

// TestCompile_LLMLabelNotInSkillsSource — out-of-set labels discarded.
func TestCompile_LLMLabelNotInSkillsSource(t *testing.T) {
	ctx := context.Background()
	story := model.RawStory{SourceFile: "a.md", Text: "s"}
	stub := llmstub.New(map[string]string{
		storyPromptKey(story.Text, skillsText): `["Go","Docker","Kubernetes"]`, // Docker not in skills
	}, 0, "")

	compiler := profilecompiler.New(stub, nil)
	profile, err := compiler.Compile(ctx, model.CompileInput{
		SkillsText: skillsText,
		Stories:    []model.RawStory{story},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, skill := range profile.Stories[0].Skills {
		if skill == "Docker" {
			t.Error("Docker in story skills; should have been filtered (not in skills source)")
		}
	}
}

// TestCompile_MalformedLLMResponse — non-JSON sets tagging_error.
func TestCompile_MalformedLLMResponse(t *testing.T) {
	ctx := context.Background()
	story := model.RawStory{SourceFile: "a.md", Text: "s"}
	stub := llmstub.New(map[string]string{
		storyPromptKey(story.Text, skillsText): `not json at all`,
	}, 0, "")

	compiler := profilecompiler.New(stub, nil)
	profile, err := compiler.Compile(ctx, model.CompileInput{
		SkillsText: skillsText,
		Stories:    []model.RawStory{story},
	})
	if err != nil {
		t.Fatalf("Compile hard error: %v", err)
	}
	if profile.Stories[0].TaggingError == "" {
		t.Error("TaggingError empty after malformed LLM response; want non-empty")
	}
	if !profile.PartialTaggingFailure {
		t.Error("PartialTaggingFailure=false after malformed response; want true")
	}
}

// helpers

func assertSkills(t *testing.T, s model.Story, want ...string) { //nolint:gocritic // hugeParam: test helper
	t.Helper()
	got := map[string]bool{}
	for _, sk := range s.Skills {
		got[sk] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("story %s missing skill %q; skills=%v", s.ID, w, s.Skills)
		}
	}
	if len(s.Skills) != len(want) {
		t.Errorf("story %s skills=%v; want exactly %v", s.ID, s.Skills, want)
	}
}

func containsSkill(s model.Story, skill string) bool { //nolint:gocritic // hugeParam: test helper
	for _, sk := range s.Skills {
		if sk == skill {
			return true
		}
	}
	return false
}

func orphanSet(orphans []model.OrphanedSkill) map[string]bool {
	m := map[string]bool{}
	for _, o := range orphans {
		m[o.Skill] = true
	}
	return m
}

// storyPromptKey is replicated from the compiler's internal prompt format.
// Tests must match the format used in compiler.go.
func init() {
	_ = strings.Contains // ensure strings imported
}
