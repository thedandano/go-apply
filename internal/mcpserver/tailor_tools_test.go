package mcpserver_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/thedandano/go-apply/internal/port"
)

// submitEditsRequest is the input shape for the planned submit_edits MCP tool.
// Defined here as a shape/contract test — the handler does not exist yet.
type submitEditsRequest struct {
	SessionID string      `json:"session_id"`
	Edits     []port.Edit `json:"edits"`
}

// validateEdit validates a single edit against the unified edit envelope contract.
func validateEdit(e port.Edit) error { //nolint:gocritic // hugeParam: test helper, pointer indirection adds no benefit here
	switch e.Op {
	case port.EditOpAdd, port.EditOpRemove, port.EditOpReplace:
		// valid op — continue
	default:
		return fmt.Errorf("invalid op %q: must be one of add, remove, replace", e.Op)
	}
	if e.Section == "experience" {
		if (e.Op == port.EditOpReplace || e.Op == port.EditOpRemove) && e.Target == "" {
			return fmt.Errorf("op %q on section %q requires a non-empty target", e.Op, e.Section)
		}
	}
	if (e.Op == port.EditOpAdd || e.Op == port.EditOpReplace) && e.Value == "" {
		return fmt.Errorf("op %q requires a non-empty value", e.Op)
	}
	return nil
}

func TestTailorTools_ValidEnvelope_Unmarshals(t *testing.T) {
	raw := []byte(`{
		"session_id": "abc123",
		"edits": [
			{"section": "skills",     "op": "replace", "value": "Go, Python"},
			{"section": "experience", "op": "replace", "target": "exp-0-b1", "value": "Led migration"}
		]
	}`)

	var req submitEditsRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if req.SessionID != "abc123" {
		t.Errorf("session_id = %q, want %q", req.SessionID, "abc123")
	}
	if len(req.Edits) != 2 {
		t.Fatalf("len(edits) = %d, want 2", len(req.Edits))
	}

	first := req.Edits[0]
	if first.Section != "skills" {
		t.Errorf("edits[0].section = %q, want %q", first.Section, "skills")
	}
	if first.Op != port.EditOpReplace {
		t.Errorf("edits[0].op = %q, want %q", first.Op, port.EditOpReplace)
	}
	if first.Target != "" {
		t.Errorf("edits[0].target = %q, want empty", first.Target)
	}
	if first.Value != "Go, Python" {
		t.Errorf("edits[0].value = %q, want %q", first.Value, "Go, Python")
	}

	second := req.Edits[1]
	if second.Section != "experience" {
		t.Errorf("edits[1].section = %q, want %q", second.Section, "experience")
	}
	if second.Op != port.EditOpReplace {
		t.Errorf("edits[1].op = %q, want %q", second.Op, port.EditOpReplace)
	}
	if second.Target != "exp-0-b1" {
		t.Errorf("edits[1].target = %q, want %q", second.Target, "exp-0-b1")
	}
	if second.Value != "Led migration" {
		t.Errorf("edits[1].value = %q, want %q", second.Value, "Led migration")
	}

	for i, edit := range req.Edits {
		if err := validateEdit(edit); err != nil {
			t.Errorf("edits[%d] unexpectedly invalid: %v", i, err)
		}
	}
}

func TestTailorTools_InvalidOp_Rejected(t *testing.T) {
	invalidCases := []struct {
		name string
		op   string
	}{
		{"empty op", ""},
		{"typo patch", "patch"},
		{"uppercase REPLACE", "REPLACE"},
		{"numeric", "1"},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			e := port.Edit{Section: "skills", Op: port.EditOp(tc.op), Value: "Go, Python"}
			if err := validateEdit(e); err == nil {
				t.Errorf("op %q: expected rejection, got nil", tc.op)
			}
		})
	}

	for _, op := range []port.EditOp{port.EditOpAdd, port.EditOpReplace} {
		e := port.Edit{Section: "skills", Op: op, Value: "Go"}
		if err := validateEdit(e); err != nil {
			t.Errorf("valid op %q unexpectedly rejected: %v", op, err)
		}
	}
	if err := validateEdit(port.Edit{Section: "skills", Op: port.EditOpRemove}); err != nil {
		t.Errorf("valid op remove on skills unexpectedly rejected: %v", err)
	}
}

func TestTailorTools_ReplaceExperienceBullet_RequiresTarget(t *testing.T) {
	t.Run("replace experience without target is rejected", func(t *testing.T) {
		e := port.Edit{Section: "experience", Op: port.EditOpReplace, Value: "Led migration"}
		if err := validateEdit(e); err == nil {
			t.Error("expected rejection for experience replace without target, got nil")
		}
	})

	t.Run("remove experience without target is rejected", func(t *testing.T) {
		e := port.Edit{Section: "experience", Op: port.EditOpRemove}
		if err := validateEdit(e); err == nil {
			t.Error("expected rejection for experience remove without target, got nil")
		}
	})

	t.Run("replace experience with target is accepted", func(t *testing.T) {
		e := port.Edit{Section: "experience", Op: port.EditOpReplace, Target: "exp-0-b1", Value: "Led migration"}
		if err := validateEdit(e); err != nil {
			t.Errorf("expected acceptance for experience replace with target, got: %v", err)
		}
	})

	t.Run("replace skills without target is accepted", func(t *testing.T) {
		e := port.Edit{Section: "skills", Op: port.EditOpReplace, Value: "Go, Python, Kubernetes"}
		if err := validateEdit(e); err != nil {
			t.Errorf("expected acceptance for skills replace without target, got: %v", err)
		}
	})
}
