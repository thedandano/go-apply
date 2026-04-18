package agentconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/thedandano/go-apply/internal/port"
)

// testEntry is the MCP server entry used across all backend tests.
var testEntry = port.MCPServerEntry{
	Command: "go-apply",
	Args:    []string{"serve"},
}

// newTestOps builds a fileOps that uses real OS functions scoped to dir.
// getenv always returns "" to keep tests isolated from the real environment.
// homeDir is set to dir; goos defaults to "linux".
func newTestOps(dir string) *fileOps {
	return &fileOps{
		readFile:   os.ReadFile,
		writeFile:  os.WriteFile,
		stat:       os.Stat,
		mkdirAll:   os.MkdirAll,
		removeAll:  os.RemoveAll,
		executable: func() (string, error) { return "/usr/local/bin/go-apply", nil },
		getenv:     func(string) string { return "" },
		homeDir:    dir,
		goos:       "linux",
	}
}

// withEnv returns a copy of ops with getenv replaced by a lookup map.
// Keys not present in the map return "".
func withEnv(ops *fileOps, env map[string]string) *fileOps {
	ops.getenv = func(key string) string { return env[key] }
	return ops
}

// writeJSON writes v as indented JSON to path, creating parent dirs as needed.
func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("writeJSON mkdir: %v", err)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("writeJSON marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("writeJSON write: %v", err)
	}
}

// writeYAML writes v as YAML to path, creating parent dirs as needed.
func writeYAML(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("writeYAML mkdir: %v", err)
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		t.Fatalf("writeYAML marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("writeYAML write: %v", err)
	}
}

// readJSON reads path and unmarshals it into map[string]any.
func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readJSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("readJSON unmarshal: %v", err)
	}
	return m
}

// readYAML reads path and unmarshals it into map[string]any.
func readYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readYAML: %v", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatalf("readYAML unmarshal: %v", err)
	}
	return m
}

// navJSON navigates root along keyPath, returning the leaf map.
func navJSON(t *testing.T, root map[string]any, keyPath []string) map[string]any {
	t.Helper()
	cur := root
	for _, k := range keyPath {
		v, ok := cur[k]
		if !ok {
			t.Fatalf("navJSON: key %q not found", k)
		}
		next, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("navJSON: key %q is not a map", k)
		}
		cur = next
	}
	return cur
}

// ---- Claude backend ---------------------------------------------------------

func TestClaudeBackend_Register_CreatesPluginDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b := newClaudeBackend(newTestOps(dir))

	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Action != port.ActionCreated {
		t.Errorf("Action = %v, want ActionCreated", res.Action)
	}
	pluginDir := filepath.Join(dir, ".claude", "plugins", "go-apply")
	if res.ConfigPath != pluginDir {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, pluginDir)
	}

	// Verify .mcp.json exists and contains expected content.
	mcpJSONPath := filepath.Join(pluginDir, ".mcp.json")
	mcpData := readJSON(t, mcpJSONPath)
	server, ok := mcpData["go-apply"].(map[string]any)
	if !ok {
		t.Fatalf(".mcp.json missing go-apply key")
	}
	if server["command"] != "/usr/local/bin/go-apply" {
		t.Errorf("command = %q, want /usr/local/bin/go-apply", server["command"])
	}

	// Verify .claude-plugin/plugin.json exists and contains expected content.
	pluginJSONPath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
	pluginData := readJSON(t, pluginJSONPath)
	if pluginData["name"] != "go-apply" {
		t.Errorf("plugin.json name = %q, want go-apply", pluginData["name"])
	}
	if pluginData["description"] == "" {
		t.Error("plugin.json description should not be empty")
	}
	author, ok := pluginData["author"].(map[string]any)
	if !ok {
		t.Fatal("plugin.json author should be a map")
	}
	if author["name"] == "" {
		t.Error("plugin.json author.name should not be empty")
	}
}

func TestClaudeBackend_Register_AlreadyRegistered(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)

	// Pre-create the .mcp.json file.
	mcpJSONPath := filepath.Join(dir, ".claude", "plugins", "go-apply", ".mcp.json")
	writeJSON(t, mcpJSONPath, map[string]any{
		"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
	})

	b := newClaudeBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Action != port.ActionAlreadyRegistered {
		t.Errorf("Action = %v, want ActionAlreadyRegistered", res.Action)
	}
}

func TestClaudeBackend_Unregister_RemovesPluginDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)

	// Pre-create plugin dir with files.
	pluginDir := filepath.Join(dir, ".claude", "plugins", "go-apply")
	mcpJSONPath := filepath.Join(pluginDir, ".mcp.json")
	writeJSON(t, mcpJSONPath, map[string]any{
		"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
	})

	b := newClaudeBackend(ops)
	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionRemoved {
		t.Errorf("Action = %v, want ActionRemoved", res.Action)
	}
	if _, statErr := os.Stat(pluginDir); statErr == nil {
		t.Error("plugin dir should be gone after Unregister")
	}
}

func TestClaudeBackend_Unregister_CleansStaleMcpServersEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)

	// Pre-create settings.json with mcpServers.go-apply entry.
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	writeJSON(t, settingsPath, map[string]any{
		"mcpServers": map[string]any{
			"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
		},
	})

	// Pre-create plugin dir.
	pluginDir := filepath.Join(dir, ".claude", "plugins", "go-apply")
	mcpJSONPath := filepath.Join(pluginDir, ".mcp.json")
	writeJSON(t, mcpJSONPath, map[string]any{
		"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
	})

	b := newClaudeBackend(ops)
	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionRemoved {
		t.Errorf("Action = %v, want ActionRemoved", res.Action)
	}

	// Plugin dir should be gone.
	if _, statErr := os.Stat(pluginDir); statErr == nil {
		t.Error("plugin dir should be gone after Unregister")
	}

	// settings.json should no longer have go-apply in mcpServers.
	root := readJSON(t, settingsPath)
	if servers, ok := root["mcpServers"].(map[string]any); ok {
		if _, found := servers["go-apply"]; found {
			t.Error("go-apply should be removed from settings.json mcpServers")
		}
	}
}

func TestClaudeBackend_Unregister_NotRegistered(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b := newClaudeBackend(newTestOps(dir))

	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionNotFound {
		t.Errorf("Action = %v, want ActionNotFound", res.Action)
	}
}

// ---- Claude plugin path resolution ------------------------------------------

func TestClaudeBackend_PluginPath_UsesHomeDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b := newClaudeBackend(newTestOps(dir))

	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	wantDir := filepath.Join(dir, ".claude", "plugins", "go-apply")
	if res.ConfigPath != wantDir {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, wantDir)
	}
}

// ---- OpenClaw backend -------------------------------------------------------

func TestOpenclawBackend_Register(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setup      func(t *testing.T, configPath string)
		wantAction port.RegistrationAction
		verify     func(t *testing.T, res port.RegistrationResult, configPath string)
	}{
		{
			name:       "CreatesNewFile",
			setup:      func(_ *testing.T, _ string) {},
			wantAction: port.ActionCreated,
			verify: func(t *testing.T, res port.RegistrationResult, configPath string) {
				if res.ConfigPath != configPath {
					t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, configPath)
				}
				root := readJSON(t, configPath)
				leaf := navJSON(t, root, []string{"mcp", "servers"})
				if _, ok := leaf["go-apply"]; !ok {
					t.Error("go-apply not found in mcp.servers")
				}
			},
		},
		{
			name: "MergesExistingFile",
			setup: func(t *testing.T, configPath string) {
				writeJSON(t, configPath, map[string]any{"other": true})
			},
			wantAction: port.ActionAdded,
			verify: func(t *testing.T, _ port.RegistrationResult, configPath string) {
				root := readJSON(t, configPath)
				if _, ok := root["other"]; !ok {
					t.Error("other key should be preserved")
				}
			},
		},
		{
			name: "AlreadyRegistered",
			setup: func(t *testing.T, configPath string) {
				writeJSON(t, configPath, map[string]any{
					"mcp": map[string]any{
						"servers": map[string]any{
							"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
						},
					},
				})
			},
			wantAction: port.ActionAlreadyRegistered,
			verify:     func(_ *testing.T, _ port.RegistrationResult, _ string) {},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			ops := newTestOps(dir)
			configPath := filepath.Join(dir, ".openclaw", "openclaw.json")
			tc.setup(t, configPath)

			b := newOpenclawBackend(ops)
			res, err := b.Register("go-apply", testEntry)
			if err != nil {
				t.Fatalf("Register: %v", err)
			}
			if res.Action != tc.wantAction {
				t.Errorf("Action = %v, want %v", res.Action, tc.wantAction)
			}
			tc.verify(t, res, configPath)
		})
	}
}

func TestOpenclawBackend_Unregister(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setup      func(t *testing.T, configPath string)
		wantAction port.RegistrationAction
	}{
		{
			name: "RemovesEntry",
			setup: func(t *testing.T, configPath string) {
				writeJSON(t, configPath, map[string]any{
					"mcp": map[string]any{
						"servers": map[string]any{
							"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
						},
					},
				})
			},
			wantAction: port.ActionRemoved,
		},
		{
			name: "EntryNotPresent",
			setup: func(t *testing.T, configPath string) {
				writeJSON(t, configPath, map[string]any{"mcp": map[string]any{"servers": map[string]any{}}})
			},
			wantAction: port.ActionNotFound,
		},
		{
			name:       "FileNotExist",
			setup:      func(_ *testing.T, _ string) {},
			wantAction: port.ActionNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			ops := newTestOps(dir)
			configPath := filepath.Join(dir, ".openclaw", "openclaw.json")
			tc.setup(t, configPath)

			b := newOpenclawBackend(ops)
			res, err := b.Unregister("go-apply")
			if err != nil {
				t.Fatalf("Unregister: %v", err)
			}
			if res.Action != tc.wantAction {
				t.Errorf("Action = %v, want %v", res.Action, tc.wantAction)
			}
		})
	}
}

// ---- OpenClaw path resolution -----------------------------------------------

func TestOpenclawBackend_PathResolution(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		buildOps       func(t *testing.T, dir string) *fileOps
		wantConfigPath func(dir string) string
	}{
		{
			name: "EnvVarTakesPrecedence",
			buildOps: func(_ *testing.T, dir string) *fileOps {
				customPath := filepath.Join(dir, "custom", "openclaw.json")
				return withEnv(newTestOps(dir), map[string]string{
					"OPENCLAW_CONFIG_PATH": customPath,
				})
			},
			wantConfigPath: func(dir string) string {
				return filepath.Join(dir, "custom", "openclaw.json")
			},
		},
		{
			name: "StateDirEnvVar",
			buildOps: func(_ *testing.T, dir string) *fileOps {
				stateDir := filepath.Join(dir, "state")
				return withEnv(newTestOps(dir), map[string]string{
					"OPENCLAW_STATE_DIR": stateDir,
				})
			},
			wantConfigPath: func(dir string) string {
				return filepath.Join(dir, "state", "openclaw.json")
			},
		},
		{
			// No env vars set; only legacy path exists.
			name: "LegacyPath",
			buildOps: func(t *testing.T, dir string) *fileOps {
				legacyPath := filepath.Join(dir, ".clawdbot", "clawdbot.json")
				writeJSON(t, legacyPath, map[string]any{})
				return newTestOps(dir)
			},
			wantConfigPath: func(dir string) string {
				return filepath.Join(dir, ".clawdbot", "clawdbot.json")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			ops := tc.buildOps(t, dir)

			b := newOpenclawBackend(ops)
			res, err := b.Register("go-apply", testEntry)
			if err != nil {
				t.Fatalf("Register: %v", err)
			}
			wantPath := tc.wantConfigPath(dir)
			if res.ConfigPath != wantPath {
				t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, wantPath)
			}
		})
	}
}

// ---- Hermes backend ---------------------------------------------------------

func TestHermesBackend_Register(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setup      func(t *testing.T, configPath string)
		wantAction port.RegistrationAction
		verify     func(t *testing.T, res port.RegistrationResult, configPath string)
	}{
		{
			// getenv returns "" by default — HERMES_HOME not set.
			name:       "CreatesNewFile",
			setup:      func(_ *testing.T, _ string) {},
			wantAction: port.ActionCreated,
			verify: func(t *testing.T, res port.RegistrationResult, configPath string) {
				if res.ConfigPath != configPath {
					t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, configPath)
				}
				root := readYAML(t, configPath)
				leaf := navJSON(t, root, []string{"mcp_servers"})
				if _, ok := leaf["go-apply"]; !ok {
					t.Error("go-apply not found in mcp_servers")
				}
			},
		},
		{
			name: "MergesExistingFile",
			setup: func(t *testing.T, configPath string) {
				writeYAML(t, configPath, map[string]any{"other": true})
			},
			wantAction: port.ActionAdded,
			verify: func(t *testing.T, _ port.RegistrationResult, configPath string) {
				root := readYAML(t, configPath)
				if _, ok := root["other"]; !ok {
					t.Error("other key should be preserved")
				}
			},
		},
		{
			name: "AlreadyRegistered",
			setup: func(t *testing.T, configPath string) {
				writeYAML(t, configPath, map[string]any{
					"mcp_servers": map[string]any{
						"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
					},
				})
			},
			wantAction: port.ActionAlreadyRegistered,
			verify:     func(_ *testing.T, _ port.RegistrationResult, _ string) {},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			ops := newTestOps(dir)
			configPath := filepath.Join(dir, ".hermes", "config.yaml")
			tc.setup(t, configPath)

			b := newHermesBackend(ops)
			res, err := b.Register("go-apply", testEntry)
			if err != nil {
				t.Fatalf("Register: %v", err)
			}
			if res.Action != tc.wantAction {
				t.Errorf("Action = %v, want %v", res.Action, tc.wantAction)
			}
			tc.verify(t, res, configPath)
		})
	}
}

func TestHermesBackend_Unregister(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		setup      func(t *testing.T, configPath string)
		wantAction port.RegistrationAction
		verify     func(t *testing.T, configPath string)
	}{
		{
			name: "RemovesEntry",
			setup: func(t *testing.T, configPath string) {
				writeYAML(t, configPath, map[string]any{
					"mcp_servers": map[string]any{
						"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
					},
				})
			},
			wantAction: port.ActionRemoved,
			verify: func(t *testing.T, configPath string) {
				root := readYAML(t, configPath)
				leaf := navJSON(t, root, []string{"mcp_servers"})
				if _, ok := leaf["go-apply"]; ok {
					t.Error("go-apply should be absent after Unregister")
				}
			},
		},
		{
			name: "EntryNotPresent",
			setup: func(t *testing.T, configPath string) {
				writeYAML(t, configPath, map[string]any{"mcp_servers": map[string]any{}})
			},
			wantAction: port.ActionNotFound,
			verify:     func(_ *testing.T, _ string) {},
		},
		{
			name:       "FileNotExist",
			setup:      func(_ *testing.T, _ string) {},
			wantAction: port.ActionNotFound,
			verify:     func(_ *testing.T, _ string) {},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			ops := newTestOps(dir)
			configPath := filepath.Join(dir, ".hermes", "config.yaml")
			tc.setup(t, configPath)

			b := newHermesBackend(ops)
			res, err := b.Unregister("go-apply")
			if err != nil {
				t.Fatalf("Unregister: %v", err)
			}
			if res.Action != tc.wantAction {
				t.Errorf("Action = %v, want %v", res.Action, tc.wantAction)
			}
			tc.verify(t, configPath)
		})
	}
}

// ---- Hermes path resolution -------------------------------------------------

func TestHermesBackend_PathResolution_HermesHomeEnvVar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	hermesHome := filepath.Join(dir, "hermes-home")
	ops := withEnv(newTestOps(dir), map[string]string{
		"HERMES_HOME": hermesHome,
	})

	b := newHermesBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	wantPath := filepath.Join(hermesHome, "config.yaml")
	if res.ConfigPath != wantPath {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, wantPath)
	}
}

// ---- Registry factory -------------------------------------------------------

func TestNewRegistrar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		agent  string
		verify func(t *testing.T, r port.AgentConfigRegistrar)
	}{
		{
			agent: "claude",
			verify: func(t *testing.T, r port.AgentConfigRegistrar) {
				if _, ok := r.(*claudeBackend); !ok {
					t.Errorf("expected *claudeBackend, got %T", r)
				}
			},
		},
		{
			agent: "openclaw",
			verify: func(t *testing.T, r port.AgentConfigRegistrar) {
				if _, ok := r.(*openclawBackend); !ok {
					t.Errorf("expected *openclawBackend, got %T", r)
				}
			},
		},
		{
			agent: "hermes",
			verify: func(t *testing.T, r port.AgentConfigRegistrar) {
				if _, ok := r.(*hermesBackend); !ok {
					t.Errorf("expected *hermesBackend, got %T", r)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.agent, func(t *testing.T) {
			t.Parallel()
			r, err := NewRegistrar(tc.agent)
			if err != nil {
				t.Fatalf("NewRegistrar(%q): %v", tc.agent, err)
			}
			tc.verify(t, r)
		})
	}
}

func TestNewRegistrar_UnknownAgent(t *testing.T) {
	t.Parallel()
	_, err := NewRegistrar("foo")
	if err == nil {
		t.Fatal("expected error for unknown agent")
	}
	if !strings.Contains(err.Error(), "valid agents are claude, openclaw, hermes") {
		t.Errorf("error message %q should contain 'valid agents are claude, openclaw, hermes'", err.Error())
	}
}

// ---- RegisterForce tests ----------------------------------------------------

func TestClaudeBackend_RegisterForce_OverwritesExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)

	// Pre-create .mcp.json with stale command path.
	pluginDir := filepath.Join(dir, ".claude", "plugins", "go-apply")
	mcpJSONPath := filepath.Join(pluginDir, ".mcp.json")
	writeJSON(t, mcpJSONPath, map[string]any{
		"go-apply": map[string]any{"command": "/old/path/go-apply", "args": []string{"serve"}},
	})

	b := newClaudeBackend(ops)
	res, err := b.RegisterForce("go-apply", testEntry)
	if err != nil {
		t.Fatalf("RegisterForce: %v", err)
	}
	if res.Action != port.ActionCreated {
		t.Errorf("Action = %v, want ActionCreated", res.Action)
	}

	// Verify .mcp.json was overwritten with the new binary path.
	mcpData := readJSON(t, mcpJSONPath)
	server, ok := mcpData["go-apply"].(map[string]any)
	if !ok {
		t.Fatalf(".mcp.json missing go-apply key after RegisterForce")
	}
	if server["command"] != "/usr/local/bin/go-apply" {
		t.Errorf("command = %q, want /usr/local/bin/go-apply", server["command"])
	}
}

func TestOpenclawBackend_RegisterForce_OverwritesExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".openclaw", "openclaw.json")
	writeJSON(t, configPath, map[string]any{
		"mcp": map[string]any{
			"servers": map[string]any{
				"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
			},
		},
	})

	b := newOpenclawBackend(ops)
	res, err := b.RegisterForce("go-apply", testEntry)
	if err != nil {
		t.Fatalf("RegisterForce: %v", err)
	}
	// Force write should succeed (not return AlreadyRegistered).
	if res.Action == port.ActionAlreadyRegistered {
		t.Errorf("RegisterForce returned ActionAlreadyRegistered, expected write action")
	}
	root := readJSON(t, configPath)
	leaf := navJSON(t, root, []string{"mcp", "servers"})
	if _, ok := leaf["go-apply"]; !ok {
		t.Error("go-apply not found in mcp.servers after RegisterForce")
	}
}

func TestHermesBackend_RegisterForce_OverwritesExisting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".hermes", "config.yaml")
	writeYAML(t, configPath, map[string]any{
		"mcp_servers": map[string]any{
			"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
		},
	})

	b := newHermesBackend(ops)
	res, err := b.RegisterForce("go-apply", testEntry)
	if err != nil {
		t.Fatalf("RegisterForce: %v", err)
	}
	if res.Action == port.ActionAlreadyRegistered {
		t.Errorf("RegisterForce returned ActionAlreadyRegistered, expected write action")
	}
	root := readYAML(t, configPath)
	leaf := navJSON(t, root, []string{"mcp_servers"})
	if _, ok := leaf["go-apply"]; !ok {
		t.Error("go-apply not found in mcp_servers after RegisterForce")
	}
}
