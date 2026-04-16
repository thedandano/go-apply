package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/thedandano/go-apply/internal/config"
	"github.com/thedandano/go-apply/internal/model"
)

const updateCacheFile = "update-check.json"

// CachePath returns the canonical path for the update check cache file.
func CachePath() string {
	return filepath.Join(config.StateDir(), updateCacheFile)
}

// ReadCache reads the update cache from path.
// Returns nil (not an error) if the file does not exist.
func ReadCache(path string) (*model.UpdateCache, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is derived from XDG_STATE_HOME/go-apply, not user input
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read update cache %s: %w", path, err)
	}
	var cache model.UpdateCache
	if err := json.Unmarshal(data, &cache); err != nil {
		// Treat a corrupt cache as absent.
		return nil, nil
	}
	return &cache, nil
}

// WriteCache atomically writes cache to path.
// Creates parent directories if they do not exist.
func WriteCache(path string, cache *model.UpdateCache) error {
	if err := os.MkdirAll(filepath.Dir(path), config.DirPerm); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("marshal update cache: %w", err)
	}
	// Write to a sibling temp file, then rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, config.FilePerm); err != nil { // #nosec G306 -- explicit FilePerm used
		return fmt.Errorf("write temp cache %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("rename cache %s → %s: %w", tmp, path, err)
	}
	return nil
}

// IsCacheFresh reports whether the cache was populated within the given TTL.
func IsCacheFresh(cache *model.UpdateCache, ttl time.Duration) bool {
	if cache == nil {
		return false
	}
	return time.Since(cache.CheckedAt) < ttl
}
