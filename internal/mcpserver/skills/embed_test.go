package skills

import (
	"strings"
	"testing"
)

func TestPromptBody_NonEmpty(t *testing.T) {
	body := PromptBody()
	if strings.TrimSpace(body) == "" {
		t.Fatal("PromptBody() returned empty; vendored resume-tailor.md is missing or blank")
	}
}

func TestPromptBody_ContainsGoldenRuleCanary(t *testing.T) {
	body := PromptBody()
	if !strings.Contains(body, "Golden Rule") {
		t.Error("PromptBody() is missing the 'Golden Rule' canary string; vendored skill body may be truncated or from the wrong source")
	}
}

func TestMustBeLoaded_PassesOnRealBody(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MustBeLoaded() panicked with a valid embedded body: %v", r)
		}
	}()
	MustBeLoaded()
}
