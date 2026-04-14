package port

import "github.com/thedandano/go-apply/internal/model"

// ApplicationRepository persists ApplicationRecords keyed by URL.
// It serves as both a JD cache (raw text + extracted data) and an
// audit trail (scores, tailoring, submission metadata, outcome).
// List() enables batch rescoring when the scorer logic is updated.
type ApplicationRepository interface {
	// Get retrieves the record for the given URL.
	// Returns (nil, false, nil) if no record exists.
	// Returns a non-nil error if a record exists but cannot be read or parsed.
	Get(url string) (*model.ApplicationRecord, bool, error)

	// Put writes a new record. Overwrites if a record for the URL already exists.
	Put(record *model.ApplicationRecord) error

	// Update replaces the full record for record.URL.
	// Returns an error if no record exists for that URL.
	Update(record *model.ApplicationRecord) error

	// List returns all stored records in undefined order.
	// Used for batch rescoring.
	List() ([]*model.ApplicationRecord, error)
}
