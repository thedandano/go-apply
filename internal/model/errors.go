package model

import (
	"errors"
	"fmt"
)

var (
	ErrSectionsMissing          = errors.New("sections missing: no sections file found for this resume")
	ErrSchemaVersionUnsupported = errors.New("sections schema version unsupported")
	ErrNotSupportedInMCPMode    = errors.New("operation not supported in MCP mode")

	// Compiled profile errors.
	ErrProfileMissing        = errors.New("compiled profile not found — run compile_profile first")
	ErrProfileSchemaMismatch = errors.New("compiled profile schema version not supported")
	ErrUnevidencedSkill      = errors.New("skill has no supporting story in compiled profile")
)

// SchemaError reports a validation failure for a specific field in the SectionMap.
type SchemaError struct {
	Field  string
	Reason string
}

func (e SchemaError) Error() string {
	return fmt.Sprintf("schema error: field %q: %s", e.Field, e.Reason)
}
