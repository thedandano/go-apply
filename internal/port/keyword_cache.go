package port

import "context"

// KeywordCacheRepository caches keyword→embedding vector mappings.
// Avoids repeated embedding API calls for the same JD keyword.
// Key is the keyword string; value is the float32 embedding vector.
type KeywordCacheRepository interface {
	// GetVector returns the cached vector for keyword, or (nil, false, nil) on cache miss.
	GetVector(ctx context.Context, keyword string) ([]float32, bool, error)
	// SetVector stores the vector for keyword. Overwrites existing entry.
	SetVector(ctx context.Context, keyword string, vector []float32) error
}
