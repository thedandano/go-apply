package model

import (
	"errors"
	"fmt"
)

var (
	ErrSectionsMissing          = errors.New("sections missing: no sections file found for this resume")
	ErrSchemaVersionUnsupported = errors.New("sections schema version unsupported")
	ErrNotSupportedInMCPMode    = errors.New("operation not supported in MCP mode")
)

// SchemaError reports a validation failure for a specific field in the SectionMap.
type SchemaError struct {
	Field  string
	Reason string
}

func (e SchemaError) Error() string {
	return fmt.Sprintf("schema error: field %q: %s", e.Field, e.Reason)
}
