package fs

import (
	"crypto/md5" // #nosec G501 -- cache key only, not security-sensitive
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

var _ port.JDCacheRepository = (*JDCacheRepository)(nil)

// cacheEntry is the on-disk JSON structure for a cached job description.
type cacheEntry struct {
	URL     string       `json:"url"`
	RawText string       `json:"raw_text"`
	JD      model.JDData `json:"jd"`
}

// JDCacheRepository stores job description cache entries as JSON files on disk.
// One file per URL, named by MD5 hex of the URL.
type JDCacheRepository struct {
	dir string
}

// NewJDCacheRepository creates a JDCacheRepository that stores files in dir.
// The directory is created on first Put if it does not exist.
func NewJDCacheRepository(dir string) *JDCacheRepository {
	return &JDCacheRepository{dir: dir}
}

// urlToFilename converts a URL to a filesystem-safe filename using MD5.
func urlToFilename(url string) string {
	sum := md5.Sum([]byte(url)) // #nosec G401 G501 -- cache key, not a security use
	return hex.EncodeToString(sum[:]) + ".json"
}

// Get returns the cached entry for url. Returns found=false (no error) if not cached.
func (r *JDCacheRepository) Get(url string) (string, model.JDData, bool) {
	path := filepath.Join(r.dir, urlToFilename(url))
	data, err := os.ReadFile(path) // #nosec G304 -- path derived from controlled dir + MD5 hash
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", model.JDData{}, false
		}
		return "", model.JDData{}, false
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return "", model.JDData{}, false
	}
	return entry.RawText, entry.JD, true
}

// Put writes a new cache entry for url atomically (write-then-rename).
// It creates r.dir if it does not exist.
func (r *JDCacheRepository) Put(url string, rawText string, jd model.JDData) error {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return fmt.Errorf("jd cache put %s: create dir: %w", url, err)
	}

	entry := cacheEntry{URL: url, RawText: rawText, JD: jd}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("jd cache put %s: marshal: %w", url, err)
	}

	if err := atomicWrite(r.dir, urlToFilename(url), data); err != nil {
		return fmt.Errorf("jd cache put %s: %w", url, err)
	}
	return nil
}

// Update reads the existing cache entry for url, replaces only the JD field, and writes back.
func (r *JDCacheRepository) Update(url string, jd model.JDData) error {
	rawText, _, found := r.Get(url)
	if !found {
		return fmt.Errorf("jd cache update %s: entry not found", url)
	}

	entry := cacheEntry{URL: url, RawText: rawText, JD: jd}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("jd cache update %s: marshal: %w", url, err)
	}

	if err := atomicWrite(r.dir, urlToFilename(url), data); err != nil {
		return fmt.Errorf("jd cache update %s: %w", url, err)
	}
	return nil
}

// atomicWrite writes data to dir/filename atomically using a temp file + rename.
// The temp file is created in dir so rename stays on the same filesystem.
func atomicWrite(dir, filename string, data []byte) error {
	tmp, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	dest := filepath.Join(dir, filename)
	if err := os.Rename(tmpPath, dest); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename to %s: %w", dest, err)
	}
	return nil
}
