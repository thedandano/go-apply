package agentconfig_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/thedandano/go-apply/internal/port"
	"github.com/thedandano/go-apply/internal/service/agentconfig"
	"gopkg.in/yaml.v3"
)

var testEntry = port.MCPServerEntry{
	Command: "go-apply",
	Args:    []string{"serve"},
}

// ---- MergeJSON tests -------------------------------------------------------

func TestMergeJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		existing          []byte
		keyPath           []string
		serverName        string
		entry             port.MCPServerEntry
		wantAlready       bool
		wantErr           bool
		checkKeys         []string // top-level JSON keys that must be present
		checkAbsentKeys   []string // top-level JSON keys that must be absent
		checkServerExists bool     // serverName must be in the leaf map
	}{
		{
			name:              "nil input creates doc with entry",
			existing:          nil,
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:              "empty bytes creates doc with entry",
			existing:          []byte{},
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:              "other top-level keys preserved",
			existing:          []byte(`{"other":true}`),
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkKeys:         []string{"other"},
			checkServerExists: true,
		},
		{
			name:              "other MCP servers preserved",
			existing:          []byte(`{"mcpServers":{"other":{}}}`),
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:        "already registered same cmd+args",
			existing:    []byte(`{"mcpServers":{"go-apply":{"command":"go-apply","args":["serve"]}}}`),
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			entry:       testEntry,
			wantAlready: true,
		},
		{
			name:              "registered with different command overwrites",
			existing:          []byte(`{"mcpServers":{"go-apply":{"command":"old","args":["serve"]}}}`),
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:              "nested 2-level keyPath creates nested structure",
			existing:          []byte(`{}`),
			keyPath:           []string{"mcp", "servers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:              "nested partial structure creates servers inside mcp",
			existing:          []byte(`{"mcp":{}}`),
			keyPath:           []string{"mcp", "servers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:       "invalid JSON returns error",
			existing:   []byte(`{corrupt`),
			keyPath:    []string{"mcpServers"},
			serverName: "go-apply",
			entry:      testEntry,
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, alreadyRegistered, err := agentconfig.MergeJSON(tc.existing, tc.keyPath, tc.serverName, tc.entry)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if alreadyRegistered != tc.wantAlready {
				t.Errorf("alreadyRegistered = %v, want %v", alreadyRegistered, tc.wantAlready)
			}

			// Idempotency: if alreadyRegistered, bytes must equal original.
			if alreadyRegistered {
				if !bytes.Equal(got, tc.existing) {
					t.Error("expected original bytes returned unchanged when alreadyRegistered=true")
				}
				return
			}

			// Must be valid JSON.
			var root map[string]any
			if err := json.Unmarshal(got, &root); err != nil {
				t.Fatalf("output is not valid JSON: %v\n%s", err, got)
			}

			// Check preserved top-level keys.
			for _, k := range tc.checkKeys {
				if _, ok := root[k]; !ok {
					t.Errorf("expected top-level key %q to be preserved", k)
				}
			}

			// Navigate keyPath and assert serverName is in the leaf map.
			if tc.checkServerExists {
				leaf := root
				for _, k := range tc.keyPath {
					v, ok := leaf[k]
					if !ok {
						t.Fatalf("key %q missing at path", k)
					}
					next, ok := v.(map[string]any)
					if !ok {
						t.Fatalf("value at key %q is not a map", k)
					}
					leaf = next
				}
				if _, ok := leaf[tc.serverName]; !ok {
					t.Errorf("server %q not found in leaf map", tc.serverName)
				}
			}
		})
	}
}

// ---- MergeYAML tests -------------------------------------------------------

func TestMergeYAML(t *testing.T) {
	t.Parallel()

	mustYAML := func(v any) []byte {
		b, err := yaml.Marshal(v)
		if err != nil {
			panic(err)
		}
		return b
	}

	cases := []struct {
		name              string
		existing          []byte
		keyPath           []string
		serverName        string
		entry             port.MCPServerEntry
		wantAlready       bool
		wantErr           bool
		checkServerExists bool
		checkKeys         []string
	}{
		{
			name:              "nil input creates doc with entry",
			existing:          nil,
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:              "empty bytes creates doc with entry",
			existing:          []byte{},
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:              "other top-level keys preserved",
			existing:          mustYAML(map[string]any{"other": true}),
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkKeys:         []string{"other"},
			checkServerExists: true,
		},
		{
			name:              "other MCP servers preserved",
			existing:          mustYAML(map[string]any{"mcpServers": map[string]any{"other": map[string]any{}}}),
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name: "already registered same cmd+args",
			existing: mustYAML(map[string]any{
				"mcpServers": map[string]any{
					"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
				},
			}),
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			entry:       testEntry,
			wantAlready: true,
		},
		{
			name: "registered with different command overwrites",
			existing: mustYAML(map[string]any{
				"mcpServers": map[string]any{
					"go-apply": map[string]any{"command": "old", "args": []string{"serve"}},
				},
			}),
			keyPath:           []string{"mcpServers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:              "nested 2-level keyPath creates nested structure",
			existing:          mustYAML(map[string]any{}),
			keyPath:           []string{"mcp", "servers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:              "nested partial structure creates servers inside mcp",
			existing:          mustYAML(map[string]any{"mcp": map[string]any{}}),
			keyPath:           []string{"mcp", "servers"},
			serverName:        "go-apply",
			entry:             testEntry,
			wantAlready:       false,
			checkServerExists: true,
		},
		{
			name:       "invalid YAML returns error",
			existing:   []byte(":\t: bad"),
			keyPath:    []string{"mcpServers"},
			serverName: "go-apply",
			entry:      testEntry,
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, alreadyRegistered, err := agentconfig.MergeYAML(tc.existing, tc.keyPath, tc.serverName, tc.entry)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if alreadyRegistered != tc.wantAlready {
				t.Errorf("alreadyRegistered = %v, want %v", alreadyRegistered, tc.wantAlready)
			}

			if alreadyRegistered {
				// For YAML, structural equality (formatting may differ).
				var wantMap, gotMap map[string]any
				_ = yaml.Unmarshal(tc.existing, &wantMap)
				_ = yaml.Unmarshal(got, &gotMap)
				wantJSON, _ := json.Marshal(wantMap)
				gotJSON, _ := json.Marshal(gotMap)
				if !bytes.Equal(wantJSON, gotJSON) {
					t.Error("expected structurally equal YAML when alreadyRegistered=true")
				}
				return
			}

			// Must be valid YAML.
			var root map[string]any
			if err := yaml.Unmarshal(got, &root); err != nil {
				t.Fatalf("output is not valid YAML: %v\n%s", err, got)
			}

			// Check preserved keys.
			for _, k := range tc.checkKeys {
				if _, ok := root[k]; !ok {
					t.Errorf("expected key %q to be preserved", k)
				}
			}

			// Navigate keyPath and assert serverName is in the leaf map.
			if tc.checkServerExists {
				leaf := root
				for _, k := range tc.keyPath {
					v, ok := leaf[k]
					if !ok {
						t.Fatalf("key %q missing at path", k)
					}
					next, ok := v.(map[string]any)
					if !ok {
						t.Fatalf("value at key %q is not a map", k)
					}
					leaf = next
				}
				if _, ok := leaf[tc.serverName]; !ok {
					t.Errorf("server %q not found in leaf map", tc.serverName)
				}
			}
		})
	}
}

// ---- RemoveJSON tests -------------------------------------------------------

func TestRemoveJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		existing    []byte
		keyPath     []string
		serverName  string
		wantPresent bool
		wantErr     bool
		checkAbsent bool   // serverName should be absent after removal
		checkKey    string // a key that must still be present in the leaf map
	}{
		{
			name:        "nil input returns wasPresent=false",
			existing:    nil,
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			wantPresent: false,
		},
		{
			name:        "entry exists with other servers",
			existing:    []byte(`{"mcpServers":{"go-apply":{"command":"go-apply","args":["serve"]},"other":{}}}`),
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			wantPresent: true,
			checkAbsent: true,
			checkKey:    "other",
		},
		{
			name:        "entry doesn't exist",
			existing:    []byte(`{"mcpServers":{"other":{}}}`),
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			wantPresent: false,
		},
		{
			name:        "nested keyPath removes at nested path",
			existing:    []byte(`{"mcp":{"servers":{"go-apply":{"command":"go-apply","args":["serve"]}}}}`),
			keyPath:     []string{"mcp", "servers"},
			serverName:  "go-apply",
			wantPresent: true,
			checkAbsent: true,
		},
		{
			name:        "last entry removed leaves empty parent map",
			existing:    []byte(`{"mcpServers":{"go-apply":{"command":"go-apply","args":["serve"]}}}`),
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			wantPresent: true,
			checkAbsent: true,
		},
		{
			name:        "missing intermediate key returns wasPresent=false",
			existing:    []byte(`{"mcp":{}}`),
			keyPath:     []string{"mcp", "servers"},
			serverName:  "go-apply",
			wantPresent: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, wasPresent, err := agentconfig.RemoveJSON(tc.existing, tc.keyPath, tc.serverName)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wasPresent != tc.wantPresent {
				t.Errorf("wasPresent = %v, want %v", wasPresent, tc.wantPresent)
			}

			// If not present, bytes should be unchanged (or nil).
			if !tc.wantPresent {
				if tc.existing == nil {
					// got may be nil or empty — both fine
					return
				}
				// structurally equal
				var wantMap, gotMap map[string]any
				_ = json.Unmarshal(tc.existing, &wantMap)
				if len(got) > 0 {
					if err := json.Unmarshal(got, &gotMap); err != nil {
						t.Fatalf("output is not valid JSON: %v", err)
					}
				}
				wantJSON, _ := json.Marshal(wantMap)
				gotJSON, _ := json.Marshal(gotMap)
				if !bytes.Equal(wantJSON, gotJSON) {
					t.Error("expected unchanged bytes when wasPresent=false")
				}
				return
			}

			// Valid JSON output expected.
			var root map[string]any
			if err := json.Unmarshal(got, &root); err != nil {
				t.Fatalf("output is not valid JSON: %v\n%s", err, got)
			}

			if tc.checkAbsent {
				// Navigate to leaf and confirm serverName is absent.
				leaf := root
				for _, k := range tc.keyPath {
					v, ok := leaf[k]
					if !ok {
						// parent key removed — that's fine only if spec allows it.
						// Spec says don't clean up empty maps, so leaf key should remain.
						t.Fatalf("intermediate key %q unexpectedly missing after removal", k)
					}
					next, ok := v.(map[string]any)
					if !ok {
						t.Fatalf("value at key %q is not a map", k)
					}
					leaf = next
				}
				if _, ok := leaf[tc.serverName]; ok {
					t.Errorf("server %q should be absent after removal", tc.serverName)
				}
				if tc.checkKey != "" {
					if _, ok := leaf[tc.checkKey]; !ok {
						t.Errorf("expected sibling key %q to be preserved", tc.checkKey)
					}
				}
			}
		})
	}
}

// ---- RemoveYAML tests -------------------------------------------------------

func TestRemoveYAML(t *testing.T) {
	t.Parallel()

	mustYAML := func(v any) []byte {
		b, err := yaml.Marshal(v)
		if err != nil {
			panic(err)
		}
		return b
	}

	cases := []struct {
		name        string
		existing    []byte
		keyPath     []string
		serverName  string
		wantPresent bool
		checkAbsent bool
		checkKey    string
	}{
		{
			name:        "nil input returns wasPresent=false",
			existing:    nil,
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			wantPresent: false,
		},
		{
			name: "entry exists with other servers",
			existing: mustYAML(map[string]any{
				"mcpServers": map[string]any{
					"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
					"other":    map[string]any{},
				},
			}),
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			wantPresent: true,
			checkAbsent: true,
			checkKey:    "other",
		},
		{
			name: "entry doesn't exist",
			existing: mustYAML(map[string]any{
				"mcpServers": map[string]any{"other": map[string]any{}},
			}),
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			wantPresent: false,
		},
		{
			name: "nested keyPath removes at nested path",
			existing: mustYAML(map[string]any{
				"mcp": map[string]any{
					"servers": map[string]any{
						"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
					},
				},
			}),
			keyPath:     []string{"mcp", "servers"},
			serverName:  "go-apply",
			wantPresent: true,
			checkAbsent: true,
		},
		{
			name: "last entry removed leaves empty parent map",
			existing: mustYAML(map[string]any{
				"mcpServers": map[string]any{
					"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
				},
			}),
			keyPath:     []string{"mcpServers"},
			serverName:  "go-apply",
			wantPresent: true,
			checkAbsent: true,
		},
		{
			name:        "missing intermediate key returns wasPresent=false",
			existing:    mustYAML(map[string]any{"mcp": map[string]any{}}),
			keyPath:     []string{"mcp", "servers"},
			serverName:  "go-apply",
			wantPresent: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, wasPresent, err := agentconfig.RemoveYAML(tc.existing, tc.keyPath, tc.serverName)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wasPresent != tc.wantPresent {
				t.Errorf("wasPresent = %v, want %v", wasPresent, tc.wantPresent)
			}

			if !tc.wantPresent {
				if tc.existing == nil {
					return
				}
				var wantMap, gotMap map[string]any
				_ = yaml.Unmarshal(tc.existing, &wantMap)
				if len(got) > 0 {
					if err := yaml.Unmarshal(got, &gotMap); err != nil {
						t.Fatalf("output is not valid YAML: %v", err)
					}
				}
				wantJSON, _ := json.Marshal(wantMap)
				gotJSON, _ := json.Marshal(gotMap)
				if !bytes.Equal(wantJSON, gotJSON) {
					t.Error("expected unchanged structure when wasPresent=false")
				}
				return
			}

			var root map[string]any
			if err := yaml.Unmarshal(got, &root); err != nil {
				t.Fatalf("output is not valid YAML: %v\n%s", err, got)
			}

			if tc.checkAbsent {
				leaf := root
				for _, k := range tc.keyPath {
					v, ok := leaf[k]
					if !ok {
						t.Fatalf("intermediate key %q unexpectedly missing after removal", k)
					}
					next, ok := v.(map[string]any)
					if !ok {
						t.Fatalf("value at key %q is not a map", k)
					}
					leaf = next
				}
				if _, ok := leaf[tc.serverName]; ok {
					t.Errorf("server %q should be absent after removal", tc.serverName)
				}
				if tc.checkKey != "" {
					if _, ok := leaf[tc.checkKey]; !ok {
						t.Errorf("expected sibling key %q to be preserved", tc.checkKey)
					}
				}
			}
		})
	}
}
