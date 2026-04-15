//go:build bdd

package steps

import (
	"net/http/httptest"
)

// bddState holds all per-scenario state for the BDD test suite.
// A fresh instance is allocated in InitializeScenario for each scenario.
type bddState struct {
	// binary path (set once by buildBinary via Before hook)
	binary string

	// per-scenario isolation
	tmpHome    string
	stubServer *httptest.Server
	stubURL    string

	// last CLI/MCP invocation result
	lastOutput string
	lastError  string
	exitCode   int

	// workflow scenario state
	jdURL           string
	jdText          string
	channel         string
	accomplishments string

	// URLs fetched by the stub server (for cache scenario)
	httpRequests []string

	// previous run state (for cache scenario)
	prevOutput string
}
