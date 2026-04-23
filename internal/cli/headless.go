package cli

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/loader"
	"github.com/thedandano/go-apply/internal/repository/fs"
	"github.com/thedandano/go-apply/internal/service/fetcher"
	"github.com/thedandano/go-apply/internal/service/pipeline"
	"github.com/thedandano/go-apply/internal/service/scorer"
	"github.com/thedandano/go-apply/internal/sessionstore"
)

// cliErrorEnvelope is the flat JSON error shape written to stderr by CLI subcommands.
// It converges on the MCP envelope shape: {status, code, message}.
type cliErrorEnvelope struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeError emits a JSON error envelope to stderr and returns a non-nil error so cobra
// sets a non-zero exit code.
func writeError(code, message string) error {
	env := cliErrorEnvelope{
		Status:  "error",
		Code:    code,
		Message: message,
	}
	data, _ := json.Marshal(env)
	fmt.Fprintf(os.Stderr, "%s\n", data)
	return fmt.Errorf("%s: %s", code, message)
}

// newSessionsDir returns the path to the sessions directory under the data dir.
// Respects XDG_DATA_HOME so tests can redirect via HOME env var.
func newSessionsDir() string {
	return filepath.Join(config.DataDir(), "sessions")
}

// openDiskStore creates (or opens) the sessions directory and returns a DiskStore.
func openDiskStore() (*sessionstore.DiskStore, error) {
	dir := newSessionsDir()
	store, err := sessionstore.NewDiskStore(dir)
	if err != nil {
		return nil, fmt.Errorf("open session store: %w", err)
	}
	return store, nil
}

// loadCLIDeps loads configuration and wires all pipeline dependencies for CLI invocations.
func loadCLIDeps() (*pipeline.ApplyConfig, error) {
	log := slog.Default()

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	defaults, err := config.LoadDefaults()
	if err != nil {
		return nil, fmt.Errorf("load defaults: %w", err)
	}

	dataDir := config.DataDir()
	appRepo := fs.NewApplicationRepository(dataDir)
	resumeRepo := fs.NewResumeRepository(dataDir)
	docLoader := loader.New()

	scorerSvc := scorer.New(defaults)
	fetcherSvc := fetcher.NewFallback(defaults, log)

	_ = cfg // config loaded; API key flows through env var
	deps := &pipeline.ApplyConfig{
		Fetcher:  fetcherSvc,
		Scorer:   scorerSvc,
		Resumes:  resumeRepo,
		Loader:   docLoader,
		AppRepo:  appRepo,
		Defaults: defaults,
	}
	return deps, nil
}
