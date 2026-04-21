package config

import (
	"strings"
	"testing"
)

func TestSetField_StringField(t *testing.T) {
	c := &Config{}
	if err := c.SetField("user_name", "Alice"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	got, err := c.GetField("user_name")
	if err != nil {
		t.Fatalf("GetField: %v", err)
	}
	if got != "Alice" {
		t.Errorf("got %q, want %q", got, "Alice")
	}
}

func TestSetField_BoolField(t *testing.T) {
	c := &Config{}
	if err := c.SetField("verbose", "true"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	got, err := c.GetField("verbose")
	if err != nil {
		t.Fatalf("GetField: %v", err)
	}
	if got != "true" {
		t.Errorf("got %q, want %q", got, "true")
	}
}

func TestSetField_FloatField(t *testing.T) {
	c := &Config{}
	if err := c.SetField("years_of_experience", "7.5"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	got, err := c.GetField("years_of_experience")
	if err != nil {
		t.Fatalf("GetField: %v", err)
	}
	if got != "7.5" {
		t.Errorf("got %q, want %q", got, "7.5")
	}
}

func TestSetField_NestedField(t *testing.T) {
	c := &Config{}
	if err := c.SetField("orchestrator.model", "claude-opus-4-6"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	got, err := c.GetField("orchestrator.model")
	if err != nil {
		t.Fatalf("GetField: %v", err)
	}
	if got != "claude-opus-4-6" {
		t.Errorf("got %q, want %q", got, "claude-opus-4-6")
	}
}

func TestSetField_UnknownKey(t *testing.T) {
	c := &Config{}
	err := c.SetField("nonexistent.field", "value")
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("error %q does not contain 'unknown config key'", err.Error())
	}
}

func TestGetField_UnknownKey(t *testing.T) {
	c := &Config{}
	_, err := c.GetField("nonexistent.field")
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unknown config key") {
		t.Errorf("error %q does not contain 'unknown config key'", err.Error())
	}
}

func TestSetField_BoolField_InvalidValue(t *testing.T) {
	c := &Config{}
	err := c.SetField("verbose", "not-a-bool")
	if err == nil {
		t.Fatal("expected error for invalid bool, got nil")
	}
}

func TestSetField_FloatField_InvalidValue(t *testing.T) {
	c := &Config{}
	err := c.SetField("years_of_experience", "not-a-float")
	if err == nil {
		t.Fatal("expected error for invalid float, got nil")
	}
}

func TestAllKeys_CoversAllFields(t *testing.T) {
	keys := AllKeys()
	if len(keys) != 10 {
		t.Errorf("AllKeys() returned %d keys, want 10", len(keys))
	}

	// Each key must be settable and gettable on a zero-value Config.
	c := &Config{}
	setValues := map[string]string{
		"log_level": "info",
		"verbose":   "false",
	}
	for _, k := range keys {
		v := "0"
		if sv, ok := setValues[k]; ok {
			v = sv
		}
		if err := c.SetField(k, v); err != nil {
			t.Errorf("SetField(%q, %q) failed: %v", k, v, err)
		}
		if _, err := c.GetField(k); err != nil {
			t.Errorf("GetField(%q) failed: %v", k, err)
		}
	}
}
