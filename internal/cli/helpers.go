package cli

import (
	"fmt"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/repository/sqlite"
	"github.com/thedandano/go-apply/internal/service/llm"
)

// newSQLiteProfile opens the SQLite profile database.
// Returns an error rather than panicking — callers must handle the error gracefully.
func newSQLiteProfile(cfg *config.Config, defaults *config.AppDefaults) (port.ProfileRepository, error) {
	repo, err := sqlite.NewProfileRepository(cfg.ResolveDBPath(), cfg.ResolveEmbeddingDim())
	if err != nil {
		return nil, fmt.Errorf("open profile db %s: %w", cfg.ResolveDBPath(), err)
	}
	_ = defaults
	return repo, nil
}

// newLLMClient builds an HTTPClient targeting the orchestrator LLM endpoint.
func newLLMClient(cfg *config.Config, defaults *config.AppDefaults) *llm.HTTPClient {
	return llm.New(cfg.Orchestrator, defaults)
}

// newEmbedderClient builds an HTTPClient targeting the embedder endpoint.
func newEmbedderClient(cfg *config.Config, defaults *config.AppDefaults) *llm.HTTPClient {
	return llm.New(cfg.Embedder, defaults)
}
