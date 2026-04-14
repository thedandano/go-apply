package cli

import (
	"fmt"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/repository/sqlite"
)

// newSQLiteProfile opens the SQLite profile/keyword-cache database.
// Returns a concrete *sqlite.ProfileRepository because it satisfies both
// port.ProfileRepository and port.KeywordCacheRepository, so callers can pass
// it for both parameters without an additional type assertion.
func newSQLiteProfile(cfg *config.Config) (*sqlite.ProfileRepository, error) {
	repo, err := sqlite.NewProfileRepository(cfg.ResolveDBPath(), cfg.ResolveEmbeddingDim())
	if err != nil {
		return nil, fmt.Errorf("open profile db %s: %w", cfg.ResolveDBPath(), err)
	}
	return repo, nil
}
