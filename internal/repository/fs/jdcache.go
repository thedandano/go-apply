package fs

import (
	"crypto/md5" // #nosec G501 -- non-cryptographic use: URL-based cache filename key
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thedandano/go-apply/internal/model"
	"github.com/thedandano/go-apply/internal/port"
)

// Compile-time interface satisfaction check.
var _ port.ApplicationRepository = (*ApplicationRepository)(nil)

// ApplicationRepository persists ApplicationRecords as JSON files on disk,
// keyed by an MD5 hash of the URL (non-cryptographic, filename-safe).
// Each file lives in dataDir/applications/.
type ApplicationRepository struct {
	dir string
}

// NewApplicationRepository constructs an ApplicationRepository rooted at dataDir/applications.
func NewApplicationRepository(dataDir string) *ApplicationRepository {
	return &ApplicationRepository{dir: filepath.Join(dataDir, "applications")}
}

// recordKey returns a hex-encoded MD5 hash of url, used as the filename stem.
func recordKey(url string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(url))) // #nosec G401
}

func (r *ApplicationRepository) filePath(url string) string {
	return filepath.Join(r.dir, recordKey(url)+".json")
}

// Get retrieves the record for the given URL.
// Returns (nil, false, nil) if no record exists.
// Returns a non-nil error if a record file exists but cannot be read or parsed.
func (r *ApplicationRepository) Get(url string) (*model.ApplicationRecord, bool, error) {
	path := r.filePath(url)
	data, err := os.ReadFile(path) // #nosec G304 -- path is dir + md5 hex, no traversal possible
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("application repo: read %s: %w", url, err)
	}
	var rec model.ApplicationRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, false, fmt.Errorf("application repo: parse %s: %w", url, err)
	}
	return &rec, true, nil
}

// Put writes a new record to disk, creating the directory if needed.
// Overwrites any existing record for the same URL.
func (r *ApplicationRepository) Put(record *model.ApplicationRecord) error {
	if err := os.MkdirAll(r.dir, 0750); err != nil { // #nosec G301
		return fmt.Errorf("application repo: mkdir %s: %w", r.dir, err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("application repo: marshal %s: %w", record.URL, err)
	}
	return os.WriteFile(r.filePath(record.URL), data, 0600)
}

// Update replaces the full record for record.URL.
// Returns os.ErrNotExist if no record exists for that URL.
func (r *ApplicationRepository) Update(record *model.ApplicationRecord) error {
	path := r.filePath(record.URL)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("application repo: update %s: %w", record.URL, err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("application repo: marshal %s: %w", record.URL, err)
	}
	return os.WriteFile(path, data, 0600)
}

// List returns all stored records in undefined order.
// Used for batch rescoring.
func (r *ApplicationRepository) List() ([]*model.ApplicationRecord, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("application repo: list %s: %w", r.dir, err)
	}
	records := make([]*model.ApplicationRecord, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(r.dir, e.Name())
		data, err := os.ReadFile(path) // #nosec G304
		if err != nil {
			return nil, fmt.Errorf("application repo: read %s: %w", e.Name(), err)
		}
		var rec model.ApplicationRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("application repo: parse %s: %w", e.Name(), err)
		}
		records = append(records, &rec)
	}
	return records, nil
}
