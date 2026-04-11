package config_test

import (
	"reflect"
	"testing"

	"github.com/thedandano/go-apply/internal/config"
)

// TestDefaultsMatchJSON verifies EmbeddedDefaults() matches config/defaults.json.
// Fails CI if someone edits one and not the other.
func TestDefaultsMatchJSON(t *testing.T) {
	fromFile, err := config.LoadDefaults()
	if err != nil {
		t.Fatalf("LoadDefaults() failed: %v", err)
	}
	embedded := config.EmbeddedDefaults()
	if !reflect.DeepEqual(fromFile, embedded) {
		t.Errorf("defaults.json and EmbeddedDefaults() are out of sync.\nJSON: %+v\nEmbedded: %+v", fromFile, embedded)
	}
}
