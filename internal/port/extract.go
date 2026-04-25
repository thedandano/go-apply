package port

// Extractor converts raw bytes (e.g., from a rendered PDF) to plain text.
// The default implementation is identity: bytes-in → string-out.
// Swapping in a real PDF extractor requires no caller changes.
type Extractor interface {
	Extract(data []byte) (string, error)
}
