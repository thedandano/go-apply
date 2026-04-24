package tailor

import (
	"strings"
	"testing"

	"github.com/thedandano/go-apply/internal/port"
)

func TestApplyBulletRewrites_EmptyRewrites_ReturnsOriginal(t *testing.T) {
	text := "Some resume text"
	result, count := ApplyBulletRewrites(text, nil)
	if result != text {
		t.Errorf("expected original text, got %q", result)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestApplyBulletRewrites_NoMatch_ReturnsOriginal(t *testing.T) {
	text := "Some resume text"
	rewrites := []port.BulletRewrite{
		{Original: "not present", Replacement: "replaced"},
	}
	result, count := ApplyBulletRewrites(text, rewrites)
	if result != text {
		t.Errorf("expected original text, got %q", result)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestApplyBulletRewrites_HappyPath_TwoSubstitutions(t *testing.T) {
	text := "Led team of 5. Reduced latency by 40%."
	rewrites := []port.BulletRewrite{
		{Original: "Led team of 5", Replacement: "Led team of 10 engineers"},
		{Original: "40%", Replacement: "60%"},
	}
	result, count := ApplyBulletRewrites(text, rewrites)
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if result != "Led team of 10 engineers. Reduced latency by 60%." {
		t.Errorf("result = %q, unexpected", result)
	}
}

func TestApplyBulletRewrites_EmptyOriginalSkipped(t *testing.T) {
	text := "Some resume text"
	rewrites := []port.BulletRewrite{
		{Original: "", Replacement: "should not replace"},
		{Original: "Some", Replacement: "Replaced"},
	}
	result, count := ApplyBulletRewrites(text, rewrites)
	if count != 1 {
		t.Errorf("count = %d, want 1 (empty original must be skipped)", count)
	}
	if result != "Replaced resume text" {
		t.Errorf("result = %q, unexpected", result)
	}
}

func TestApplyBulletRewrites_AllEmpty_ReturnsOriginal(t *testing.T) {
	text := "Some resume text"
	rewrites := []port.BulletRewrite{
		{Original: "", Replacement: "x"},
		{Original: "", Replacement: "y"},
	}
	result, count := ApplyBulletRewrites(text, rewrites)
	if result != text {
		t.Errorf("expected original text, got %q", result)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

// ── ApplySkillsRewrites tests ─────────────────────────────────────────────────

func TestApplySkillsRewrites_ReplacesInsideSkillsSection(t *testing.T) {
	resume := "# Experience\n- Built CI/CD pipelines\n\n## Skills\nCloud: AWS, CI/CD\nLanguages: Go\n\n# Education\nBSc"
	rewrites := []port.BulletRewrite{
		{Original: "CI/CD", Replacement: "Apache Kafka, CI/CD"},
	}
	result, count, found := ApplySkillsRewrites(resume, rewrites)
	if !found {
		t.Fatal("expected skills_section_found=true")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if !strings.Contains(result, "Apache Kafka, CI/CD") {
		t.Error("replacement not applied inside Skills section")
	}
	// Experience must be unchanged (SC-005 / US1-Scenario2).
	if !strings.Contains(result, "- Built CI/CD pipelines") {
		t.Error("Experience bullet was modified — must not touch text outside Skills section")
	}
}

func TestApplySkillsRewrites_ScopesBoundary_ExperienceUnchanged(t *testing.T) {
	resume := "# Experience\n- CI/CD pipeline engineer\n\n## Skills\nCI/CD, Docker\n\n# Education\nBSc"
	rewrites := []port.BulletRewrite{
		{Original: "CI/CD", Replacement: "Apache Kafka, CI/CD"},
	}
	result, count, found := ApplySkillsRewrites(resume, rewrites)
	if !found {
		t.Fatal("expected found=true")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if strings.Contains(result, "Apache Kafka, CI/CD pipeline engineer") {
		t.Error("replacement bled into Experience section")
	}
	if !strings.Contains(result, "- CI/CD pipeline engineer") {
		t.Error("Experience bullet was altered")
	}
}

func TestApplySkillsRewrites_SubstitutionsCountEntryLevel(t *testing.T) {
	// Entry-level count: 1 per matched rewrite pair, regardless of occurrence count.
	resume := "## Skills\nDocker Docker Docker\n"
	rewrites := []port.BulletRewrite{
		{Original: "Docker", Replacement: "Docker, K8s"},
	}
	_, count, found := ApplySkillsRewrites(resume, rewrites)
	if !found {
		t.Fatal("expected found=true")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (entry-level, not occurrence-level)", count)
	}
}

func TestApplySkillsRewrites_SectionNotFound_ReturnsOriginalFalse(t *testing.T) {
	resume := "# Experience\n- Built systems\n\n# Education\nBSc CS"
	rewrites := []port.BulletRewrite{{Original: "Go", Replacement: "Go, Rust"}}
	result, count, found := ApplySkillsRewrites(resume, rewrites)
	if found {
		t.Error("expected found=false when no Skills section")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
	if result != resume {
		t.Error("result must equal original text when Skills section not found")
	}
}

func TestApplySkillsRewrites_EmptyOriginalSkipped(t *testing.T) {
	resume := "## Skills\nGo, Docker\n"
	rewrites := []port.BulletRewrite{
		{Original: "", Replacement: "should not replace"},
		{Original: "Go", Replacement: "Go, Rust"},
	}
	result, count, found := ApplySkillsRewrites(resume, rewrites)
	if !found {
		t.Fatal("expected found=true")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (empty original skipped)", count)
	}
	if !strings.Contains(result, "Go, Rust") {
		t.Errorf("valid rewrite not applied; result: %q", result)
	}
}

func TestApplySkillsRewrites_AllEmptyOriginal_CountZeroSectionFound(t *testing.T) {
	// Service layer: all-empty originals → count=0, section found=true.
	// Handler validates and rejects before reaching service (empty_skill_rewrites error).
	resume := "## Skills\nGo, Docker\n"
	rewrites := []port.BulletRewrite{
		{Original: "", Replacement: "x"},
		{Original: "", Replacement: "y"},
	}
	_, count, found := ApplySkillsRewrites(resume, rewrites)
	if !found {
		t.Fatal("expected found=true (section exists)")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestApplySkillsRewrites_ArrayOrderApplied(t *testing.T) {
	// FR-007: rewrites applied in submission array order.
	resume := "## Skills\nGo, Docker\n"
	rewrites := []port.BulletRewrite{
		{Original: "Go", Replacement: "Go 1.21"},
		{Original: "Go 1.21", Replacement: "Go 1.22"},
	}
	result, _, found := ApplySkillsRewrites(resume, rewrites)
	if !found {
		t.Fatal("expected found=true")
	}
	if !strings.Contains(result, "Go 1.22") {
		t.Errorf("array-order application failed; result: %q", result)
	}
	if strings.Contains(result, "Go 1.21,") {
		t.Errorf("second rewrite not applied in order; result: %q", result)
	}
}
