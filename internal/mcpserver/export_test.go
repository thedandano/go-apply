package mcpserver

import (
	"context"

	"github.com/thedandano/go-apply/internal/port"
)

// CheckFR010aForTest exposes checkFR010a for black-box testing.
// valueStr is treated as a single add-edit value (comma-separated skill tokens).
func CheckFR010aForTest(ctx context.Context, sessionID, dataDir, valueStr string) *Envelope {
	edits := []port.Edit{{Section: "skills", Op: port.EditOpAdd, Value: valueStr}}
	return checkFR010a(ctx, sessionID, edits, dataDir)
}
