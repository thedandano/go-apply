package profilecompiler_test

import (
	"context"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/service/profilecompiler"
)

func newCompiler() *profilecompiler.Compiler {
	return profilecompiler.New(nil)
}

func orphanSet(orphans []model.OrphanedSkill) map[string]bool {
	m := map[string]bool{}
	for _, o := range orphans {
		m[o.Skill] = true
	}
	return m
}

// TestCompile_SkillUnion verifies: effective_skills = prior_skills ∪ input.Skills − remove_skills, sorted.
func TestCompile_SkillUnion(t *testing.T) {
	ctx := context.Background()
	prior := &model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now(),
		Skills:        []string{"Kubernetes", "Go"},
	}
	input := model.AssembleInput{
		Skills:       []string{"Terraform"},
		RemoveSkills: []string{"Kubernetes"},
		Stories:      nil,
		PriorProfile: prior,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	want := []string{"Go", "Terraform"}
	if len(profile.Skills) != len(want) {
		t.Fatalf("skills = %v; want %v", profile.Skills, want)
	}
	for i, sk := range want {
		if profile.Skills[i] != sk {
			t.Errorf("skills[%d] = %q; want %q", i, profile.Skills[i], sk)
		}
	}
}

// TestCompile_NewStoryAssignment verifies a new story (no ID) gets assigned an ID like "story-001".
func TestCompile_NewStoryAssignment(t *testing.T) {
	ctx := context.Background()
	input := model.AssembleInput{
		Skills: []string{"Go"},
		Stories: []model.AssembleStory{
			{Accomplishment: "shipped the thing", Tags: []string{"Go"}},
		},
		PriorProfile: nil,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(profile.Stories) != 1 {
		t.Fatalf("stories count = %d; want 1", len(profile.Stories))
	}

	s := profile.Stories[0]
	if s.ID != "story-001" {
		t.Errorf("story ID = %q; want %q", s.ID, "story-001")
	}
	if s.Text != "shipped the thing" {
		t.Errorf("story Text = %q; want %q", s.Text, "shipped the thing")
	}
	if len(s.Skills) != 1 || s.Skills[0] != "Go" {
		t.Errorf("story Skills = %v; want [Go]", s.Skills)
	}
}

// TestCompile_ExistingStoryByID verifies text comes from prior and tags are replaced by input.
func TestCompile_ExistingStoryByID(t *testing.T) {
	ctx := context.Background()
	prior := &model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now(),
		Stories: []model.Story{
			{ID: "story-001", SourceFile: "acc-0.md", Text: "old text", Skills: []string{"A"}},
		},
	}
	input := model.AssembleInput{
		Skills: []string{"Go", "K8s"},
		Stories: []model.AssembleStory{
			{ID: "story-001", Tags: []string{"Go", "K8s"}},
		},
		PriorProfile: prior,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(profile.Stories) != 1 {
		t.Fatalf("stories count = %d; want 1", len(profile.Stories))
	}

	s := profile.Stories[0]
	if s.ID != "story-001" {
		t.Errorf("story ID = %q; want story-001", s.ID)
	}
	if s.Text != "old text" {
		t.Errorf("story Text = %q; want %q (text must come from prior)", s.Text, "old text")
	}
	if s.SourceFile != "" {
		t.Errorf("story SourceFile = %q; want %q (Source not set → empty)", s.SourceFile, "")
	}
	// Tags replaced by input.
	wantSkills := map[string]bool{"Go": true, "K8s": true}
	if len(s.Skills) != len(wantSkills) {
		t.Errorf("story Skills = %v; want Go+K8s", s.Skills)
	}
	for _, sk := range s.Skills {
		if !wantSkills[sk] {
			t.Errorf("unexpected skill %q in story", sk)
		}
	}
}

// TestCompile_UnknownIDError verifies that an unknown story ID returns an error and an empty profile.
func TestCompile_UnknownIDError(t *testing.T) {
	ctx := context.Background()
	prior := &model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now(),
		Stories: []model.Story{
			{ID: "story-001", Text: "first story"},
		},
	}
	input := model.AssembleInput{
		Skills: []string{"Go"},
		Stories: []model.AssembleStory{
			{ID: "story-999", Tags: []string{"Go"}},
		},
		PriorProfile: prior,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err == nil {
		t.Fatal("Compile returned nil error; want error for unknown story ID")
	}
	// Result must be the zero value.
	if profile.SchemaVersion != "" || len(profile.Stories) != 0 {
		t.Errorf("profile must be zero value on error; got %+v", profile)
	}
}

// TestCompile_NoPriorProfileWithID verifies that providing an ID with no prior profile returns an error.
func TestCompile_NoPriorProfileWithID(t *testing.T) {
	ctx := context.Background()
	input := model.AssembleInput{
		Skills: []string{"Go"},
		Stories: []model.AssembleStory{
			{ID: "story-001", Tags: []string{"Go"}},
		},
		PriorProfile: nil,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err == nil {
		t.Fatal("Compile returned nil error; want error for ID with no prior profile")
	}
	if profile.SchemaVersion != "" || len(profile.Stories) != 0 {
		t.Errorf("profile must be zero value on error; got %+v", profile)
	}
}

// TestCompile_OrphanedSkills verifies skills with no story coverage appear in OrphanedSkills.
func TestCompile_OrphanedSkills(t *testing.T) {
	ctx := context.Background()
	input := model.AssembleInput{
		Skills: []string{"Go", "Kubernetes", "Terraform"},
		Stories: []model.AssembleStory{
			{Accomplishment: "did Go things", Tags: []string{"Go"}},
		},
		PriorProfile: nil,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	orphans := orphanSet(profile.OrphanedSkills)
	if orphans["Go"] {
		t.Error("Go in OrphanedSkills; it is covered by a story")
	}
	if !orphans["Kubernetes"] {
		t.Error("Kubernetes not in OrphanedSkills; want it orphaned")
	}
	if !orphans["Terraform"] {
		t.Error("Terraform not in OrphanedSkills; want it orphaned")
	}
}

// TestCompile_NoOrphans verifies that all covered skills produce an empty OrphanedSkills.
func TestCompile_NoOrphans(t *testing.T) {
	ctx := context.Background()
	input := model.AssembleInput{
		Skills: []string{"Go", "Kubernetes"},
		Stories: []model.AssembleStory{
			{Accomplishment: "story one", Tags: []string{"Go", "Kubernetes"}},
		},
		PriorProfile: nil,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(profile.OrphanedSkills) != 0 {
		t.Errorf("OrphanedSkills = %v; want empty", profile.OrphanedSkills)
	}
}

// TestCompile_DeferredCarryForward verifies orphaned skills keep their Deferred flag from the prior.
// Also checks the negative: a non-deferred prior orphan stays non-deferred.
func TestCompile_DeferredCarryForward(t *testing.T) {
	ctx := context.Background()
	prior := &model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now(),
		OrphanedSkills: []model.OrphanedSkill{
			{Skill: "Kubernetes", Deferred: true},
			{Skill: "Terraform", Deferred: false},
		},
	}
	// Include both skills so they remain in effective set.
	input := model.AssembleInput{
		Skills: []string{"Go", "Kubernetes", "Terraform"},
		Stories: []model.AssembleStory{
			{Accomplishment: "Go work", Tags: []string{"Go"}},
		},
		PriorProfile: prior,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// Build a map from skill name to OrphanedSkill for easy lookup.
	orphanDetails := map[string]model.OrphanedSkill{}
	for _, o := range profile.OrphanedSkills {
		orphanDetails[o.Skill] = o
	}

	k8s, ok := orphanDetails["Kubernetes"]
	if !ok {
		t.Fatal("Kubernetes not in OrphanedSkills; want it present")
	}
	if !k8s.Deferred {
		t.Error("Kubernetes Deferred=false; want true (carried from prior)")
	}

	tf, ok := orphanDetails["Terraform"]
	if !ok {
		t.Fatal("Terraform not in OrphanedSkills; want it present")
	}
	if tf.Deferred {
		t.Error("Terraform Deferred=true; want false (not deferred in prior)")
	}
}

// TestCompile_NewStoryIDSequencing verifies that new stories get IDs continuing from the prior count.
func TestCompile_NewStoryIDSequencing(t *testing.T) {
	ctx := context.Background()
	prior := &model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now(),
		Stories: []model.Story{
			{ID: "story-001", Text: "first"},
			{ID: "story-002", Text: "second"},
		},
	}
	input := model.AssembleInput{
		Skills: []string{"Go"},
		Stories: []model.AssembleStory{
			{Accomplishment: "third story", Tags: []string{"Go"}},
			{Accomplishment: "fourth story", Tags: []string{"Go"}},
		},
		PriorProfile: prior,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(profile.Stories) != 2 {
		t.Fatalf("stories count = %d; want 2", len(profile.Stories))
	}
	if profile.Stories[0].ID != "story-003" {
		t.Errorf("stories[0].ID = %q; want story-003", profile.Stories[0].ID)
	}
	if profile.Stories[1].ID != "story-004" {
		t.Errorf("stories[1].ID = %q; want story-004", profile.Stories[1].ID)
	}
}

// TestCompile_RemoveSkills verifies a skill explicitly removed via RemoveSkills is absent from the
// profile skill roster but does not strip existing story tags (story content is immutable).
func TestCompile_RemoveSkills(t *testing.T) {
	ctx := context.Background()
	prior := &model.CompiledProfile{
		SchemaVersion: "1",
		CompiledAt:    time.Now(),
		Skills:        []string{"Go", "Java", "Kubernetes"},
		Stories:       []model.Story{{ID: "story-001", Text: "built a Java service", Skills: []string{"Java"}}},
	}
	input := model.AssembleInput{
		RemoveSkills: []string{"Java"},
		Stories:      []model.AssembleStory{{ID: "story-001", Tags: []string{"Java"}}},
		PriorProfile: prior,
	}

	profile, err := newCompiler().Compile(ctx, input)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	for _, sk := range profile.Skills {
		if sk == "Java" {
			t.Error("Java still in skills after explicit removal")
		}
	}
	if len(profile.Stories) == 0 || len(profile.Stories[0].Skills) == 0 || profile.Stories[0].Skills[0] != "Java" {
		t.Error("story-001 must retain its Java tag even after Java is removed from profile skills")
	}
}

// TestCompile_SourceFieldCopied verifies AssembleStory.Source is copied to Story.SourceFile.
func TestCompile_SourceFieldCopied(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name   string
		source string
		want   string
	}{
		{"onboard", "onboard", "onboard"},
		{"created_story_id", "2", "2"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := model.AssembleInput{
				Skills: []string{"Go"},
				Stories: []model.AssembleStory{
					{Accomplishment: "did the thing", Tags: []string{"Go"}, Source: tc.source},
				},
			}
			profile, err := newCompiler().Compile(ctx, input)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if len(profile.Stories) != 1 {
				t.Fatalf("stories count = %d; want 1", len(profile.Stories))
			}
			if profile.Stories[0].SourceFile != tc.want {
				t.Errorf("SourceFile = %q; want %q", profile.Stories[0].SourceFile, tc.want)
			}
		})
	}
}
