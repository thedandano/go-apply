package port

// DocumentLoader extracts plain text from a file at a given path.
// One implementation handles all formats via extension dispatch.
// Used by: ResumeRepository, onboarding (skills_reference, accomplishments),
// and any future file input. Add a new format in loader.Dispatcher only.
type DocumentLoader interface {
	// Load extracts plain text from the file at path.
	// Supports: .docx, .pdf, .md, .markdown, .txt, .text
	Load(path string) (string, error)

	// Supports returns true if this loader can handle the given file extension.
	// Extension must include the dot: ".pdf", ".docx", etc.
	Supports(ext string) bool
}
