package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Environment variable names that override the corresponding tailor defaults
// (spec FR-012, FR-016). Exported so callers and tests share one source of truth.
const (
	EnvTailorLLMEnabled     = "GO_APPLY_TAILOR_LLM_ENABLED"
	EnvTailorSessionTimeout = "GO_APPLY_TAILOR_SESSION_TIMEOUT"
)

// ResolvedTailor holds the runtime-resolved tailor configuration after
// env-var overrides are applied on top of baked defaults.
type ResolvedTailor struct {
	LLMEnabled     bool
	SessionTimeout time.Duration
}

// ResolveTailor applies env-var overrides on top of the baked TailorDefaults,
// returning the runtime values the pipeline and MCP server should use.
//
// GO_APPLY_TAILOR_LLM_ENABLED accepts any value parseable by strconv.ParseBool
// (1, 0, t, f, true, false, TRUE, FALSE, etc.). A malformed value is an error.
// GO_APPLY_TAILOR_SESSION_TIMEOUT is an integer number of seconds.
func ResolveTailor(d TailorDefaults) (ResolvedTailor, error) {
	out := ResolvedTailor{
		LLMEnabled:     d.LLMEnabled,
		SessionTimeout: time.Duration(d.SessionTimeoutSeconds) * time.Second,
	}

	if v, ok := os.LookupEnv(EnvTailorLLMEnabled); ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return ResolvedTailor{}, fmt.Errorf("parse %s=%q: %w", EnvTailorLLMEnabled, v, err)
		}
		out.LLMEnabled = b
	}

	if v, ok := os.LookupEnv(EnvTailorSessionTimeout); ok {
		n, err := strconv.Atoi(v)
		if err != nil {
			return ResolvedTailor{}, fmt.Errorf("parse %s=%q (expected integer seconds): %w", EnvTailorSessionTimeout, v, err)
		}
		if n <= 0 {
			return ResolvedTailor{}, fmt.Errorf("%s=%d: must be > 0 seconds", EnvTailorSessionTimeout, n)
		}
		out.SessionTimeout = time.Duration(n) * time.Second
	}

	return out, nil
}
