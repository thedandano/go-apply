package config

import (
	"os"
	"testing"
	"time"
)

// unsetTestEnv ensures a given env var is absent for the duration of the test.
// t.Setenv("KEY", "") keeps the variable set-to-empty (LookupEnv returns
// ok=true), which is the wrong signal for "unset". This helper clears it and
// restores the prior value on cleanup.
func unsetTestEnv(t *testing.T, key string) {
	t.Helper()
	prior, hadPrior := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadPrior {
			_ = os.Setenv(key, prior)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestResolveTailor_DefaultsWhenEnvUnset(t *testing.T) {
	unsetTestEnv(t, EnvTailorLLMEnabled)
	unsetTestEnv(t, EnvTailorSessionTimeout)

	got, err := ResolveTailor(TailorDefaults{LLMEnabled: true, SessionTimeoutSeconds: 300})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.LLMEnabled {
		t.Errorf("LLMEnabled = false, want true (default)")
	}
	if got.SessionTimeout != 300*time.Second {
		t.Errorf("SessionTimeout = %s, want 300s", got.SessionTimeout)
	}
}

func TestResolveTailor_LLMEnabledEnvOverride(t *testing.T) {
	unsetTestEnv(t, EnvTailorSessionTimeout)
	t.Setenv(EnvTailorLLMEnabled, "false")

	got, err := ResolveTailor(TailorDefaults{LLMEnabled: true, SessionTimeoutSeconds: 300})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.LLMEnabled {
		t.Errorf("LLMEnabled = true, want false (env override)")
	}
}

func TestResolveTailor_SessionTimeoutEnvOverride(t *testing.T) {
	unsetTestEnv(t, EnvTailorLLMEnabled)
	t.Setenv(EnvTailorSessionTimeout, "600")

	got, err := ResolveTailor(TailorDefaults{LLMEnabled: true, SessionTimeoutSeconds: 300})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionTimeout != 600*time.Second {
		t.Errorf("SessionTimeout = %s, want 600s (env override)", got.SessionTimeout)
	}
}

func TestResolveTailor_MalformedLLMEnabledRejected(t *testing.T) {
	unsetTestEnv(t, EnvTailorSessionTimeout)
	t.Setenv(EnvTailorLLMEnabled, "not-a-bool")

	if _, err := ResolveTailor(TailorDefaults{LLMEnabled: true, SessionTimeoutSeconds: 300}); err == nil {
		t.Fatal("expected error for malformed bool, got nil")
	}
}

func TestResolveTailor_MalformedSessionTimeoutRejected(t *testing.T) {
	unsetTestEnv(t, EnvTailorLLMEnabled)
	t.Setenv(EnvTailorSessionTimeout, "abc")

	if _, err := ResolveTailor(TailorDefaults{LLMEnabled: true, SessionTimeoutSeconds: 300}); err == nil {
		t.Fatal("expected error for non-integer timeout, got nil")
	}
}

func TestResolveTailor_ZeroOrNegativeSessionTimeoutRejected(t *testing.T) {
	unsetTestEnv(t, EnvTailorLLMEnabled)
	t.Setenv(EnvTailorSessionTimeout, "0")

	if _, err := ResolveTailor(TailorDefaults{LLMEnabled: true, SessionTimeoutSeconds: 300}); err == nil {
		t.Fatal("expected error for zero timeout, got nil")
	}
}

func TestEmbeddedDefaults_IncludesTailorLLMFields(t *testing.T) {
	d := EmbeddedDefaults()
	if !d.Tailor.LLMEnabled {
		t.Errorf("EmbeddedDefaults().Tailor.LLMEnabled = false, want true")
	}
	if d.Tailor.SessionTimeoutSeconds != 300 {
		t.Errorf("EmbeddedDefaults().Tailor.SessionTimeoutSeconds = %d, want 300", d.Tailor.SessionTimeoutSeconds)
	}
}
