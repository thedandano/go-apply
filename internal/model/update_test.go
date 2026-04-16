package model_test

import (
	"testing"

	"github.com/thedandano/go-apply/internal/model"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"patch bump", "0.1.0", "0.1.1", true},
		{"minor bump", "0.1.0", "0.2.0", true},
		{"major bump", "0.1.0", "1.0.0", true},
		{"same version", "0.1.0", "0.1.0", false},
		{"older version", "0.2.0", "0.1.0", false},
		{"v prefix on latest", "0.1.0", "v0.1.1", true},
		{"v prefix on current", "v0.1.0", "0.1.1", true},
		{"v prefix on both", "v0.1.0", "v0.2.0", true},
		{"dev current", "dev", "1.0.0", false},
		{"pre-release stripped", "0.1.0", "0.2.0-rc1", true},
		{"invalid current", "abc", "0.1.0", false},
		{"invalid latest", "0.1.0", "abc", false},
		{"two-part version", "0.1", "0.2.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.IsNewer(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}
