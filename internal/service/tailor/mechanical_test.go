package tailor

import "testing"

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
	rewrites := []BulletRewrite{
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
	rewrites := []BulletRewrite{
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
	rewrites := []BulletRewrite{
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
	rewrites := []BulletRewrite{
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
