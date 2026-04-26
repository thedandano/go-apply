package port

import "context"

// Extractor converts raw bytes (e.g., from a rendered PDF) to plain text.
// The context is used to propagate cancellation to any subprocess.
type Extractor interface {
	Extract(ctx context.Context, data []byte) (string, error)
}
