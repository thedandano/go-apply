package fs

import (
	"crypto/md5" // #nosec G501 -- non-cryptographic use: URL-based cache filename key
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.JDCacheRepository = (*JDCacheRepository)(nil)

// JDCacheRepository stores parsed job description data as JSON files on disk.
// Each entry is keyed by an MD5 hash of the URL (non-cryptographic, filename-safe).
type JDCacheRepository struct {
	cacheDir string
}

// NewJDCacheRepository constructs a JDCacheRepository rooted at dataDir/jd_cache.
func NewJDCacheRepository(dataDir string) *JDCacheRepository {
	return &JDCacheRepository{cacheDir: filepath.Join(dataDir, "jd_cache")}
}

// cacheEntry is the on-disk JSON structure for a single cached JD.
type cacheEntry struct {
	URL     string       `json:"url"`
	RawText string       `json:"raw_text"`
	JD      model.JDData `json:"jd"`
}

// cacheKey returns a hex-encoded MD5 hash of url, used as the filename.
func cacheKey(url string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(url))) // #nosec G401
}

// Get retrieves a cached JD entry by url.
// Returns found=false (no error) if the entry does not exist.
func (r *JDCacheRepository) Get(url string) (rawText string, jd model.JDData, found bool) {
	path := filepath.Join(r.cacheDir, cacheKey(url)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", model.JDData{}, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return "", model.JDData{}, false
	}
	return entry.RawText, entry.JD, true
}

// Put writes a new JD cache entry to disk, creating the cache directory if needed.
func (r *JDCacheRepository) Put(url string, rawText string, jd model.JDData) error { //nolint:gocritic // interface requires value type
	if err := os.MkdirAll(r.cacheDir, 0755); err != nil {
		return fmt.Errorf("jd cache: mkdir %s: %w", r.cacheDir, err)
	}
	entry := cacheEntry{URL: url, RawText: rawText, JD: jd}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("jd cache: marshal %s: %w", url, err)
	}
	return os.WriteFile(filepath.Join(r.cacheDir, cacheKey(url)+".json"), data, 0600)
}

// Update replaces the JD field of an existing cache entry, preserving the raw text.
// Returns os.ErrNotExist if no entry exists for the given url.
func (r *JDCacheRepository) Update(url string, jd model.JDData) error { //nolint:gocritic // interface requires value type
	path := filepath.Join(r.cacheDir, cacheKey(url)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("jd cache update %s: %w", url, err)
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return fmt.Errorf("jd cache update parse %s: %w", url, err)
	}
	entry.JD = jd
	updated, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("jd cache update marshal %s: %w", url, err)
	}
	return os.WriteFile(path, updated, 0600)
}
