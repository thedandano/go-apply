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
func newTestOps(dir string) fileOps {
	return fileOps{
		readFile:  os.ReadFile,
		writeFile: os.WriteFile,
		stat:      os.Stat,
		mkdirAll:  os.MkdirAll,
		getenv:    func(string) string { return "" },
		homeDir:   dir,
		goos:      "linux",
	}
}

// withEnv returns a copy of ops with getenv replaced by a lookup map.
// Keys not present in the map return "".
func withEnv(ops fileOps, env map[string]string) fileOps {
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

func TestClaudeBackend_Register_CreatesNewFile(t *testing.T) {
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
	configPath := filepath.Join(dir, ".claude", "settings.json")
	if res.ConfigPath != configPath {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, configPath)
	}
	root := readJSON(t, configPath)
	leaf := navJSON(t, root, []string{"mcpServers"})
	if _, ok := leaf["go-apply"]; !ok {
		t.Error("go-apply not found in mcpServers")
	}
}

func TestClaudeBackend_Register_MergesExistingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".claude", "settings.json")
	writeJSON(t, configPath, map[string]any{"other": true})

	b := newClaudeBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Action != port.ActionAdded {
		t.Errorf("Action = %v, want ActionAdded", res.Action)
	}
	root := readJSON(t, configPath)
	if _, ok := root["other"]; !ok {
		t.Error("other key should be preserved")
	}
	leaf := navJSON(t, root, []string{"mcpServers"})
	if _, ok := leaf["go-apply"]; !ok {
		t.Error("go-apply not found after merge")
	}
}

func TestClaudeBackend_Register_AlreadyRegistered(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".claude", "settings.json")
	writeJSON(t, configPath, map[string]any{
		"mcpServers": map[string]any{
			"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
		},
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

func TestClaudeBackend_Unregister_RemovesEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".claude", "settings.json")
	writeJSON(t, configPath, map[string]any{
		"mcpServers": map[string]any{
			"go-apply": map[string]any{"command": "go-apply", "args": []string{"serve"}},
		},
	})

	b := newClaudeBackend(ops)
	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionRemoved {
		t.Errorf("Action = %v, want ActionRemoved", res.Action)
	}
	root := readJSON(t, configPath)
	leaf := navJSON(t, root, []string{"mcpServers"})
	if _, ok := leaf["go-apply"]; ok {
		t.Error("go-apply should be absent after Unregister")
	}
}

func TestClaudeBackend_Unregister_EntryNotPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".claude", "settings.json")
	writeJSON(t, configPath, map[string]any{"mcpServers": map[string]any{}})

	b := newClaudeBackend(ops)
	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionNotFound {
		t.Errorf("Action = %v, want ActionNotFound", res.Action)
	}
}

func TestClaudeBackend_Unregister_FileNotExist(t *testing.T) {
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

// ---- Claude path resolution -------------------------------------------------

func TestClaudeBackend_PathResolution_CLIPreferred(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	ops.goos = "darwin"

	// Create both CLI and desktop paths.
	cliPath := filepath.Join(dir, ".claude", "settings.json")
	writeJSON(t, cliPath, map[string]any{})
	desktopPath := filepath.Join(dir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	writeJSON(t, desktopPath, map[string]any{})

	b := newClaudeBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.ConfigPath != cliPath {
		t.Errorf("ConfigPath = %q, want CLI path %q", res.ConfigPath, cliPath)
	}
}

func TestClaudeBackend_PathResolution_FallsBackToDesktopOnDarwin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	ops.goos = "darwin"

	// Only desktop path exists.
	desktopPath := filepath.Join(dir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	writeJSON(t, desktopPath, map[string]any{})

	b := newClaudeBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.ConfigPath != desktopPath {
		t.Errorf("ConfigPath = %q, want desktop path %q", res.ConfigPath, desktopPath)
	}
}

func TestClaudeBackend_PathResolution_DefaultsToCliPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	ops.goos = "linux"

	// Neither path exists — should default to CLI path.
	b := newClaudeBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	wantPath := filepath.Join(dir, ".claude", "settings.json")
	if res.ConfigPath != wantPath {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, wantPath)
	}
}

// ---- OpenClaw backend -------------------------------------------------------

func TestOpenclawBackend_Register_CreatesNewFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b := newOpenclawBackend(newTestOps(dir))

	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Action != port.ActionCreated {
		t.Errorf("Action = %v, want ActionCreated", res.Action)
	}
	configPath := filepath.Join(dir, ".openclaw", "openclaw.json")
	if res.ConfigPath != configPath {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, configPath)
	}
	root := readJSON(t, configPath)
	leaf := navJSON(t, root, []string{"mcp", "servers"})
	if _, ok := leaf["go-apply"]; !ok {
		t.Error("go-apply not found in mcp.servers")
	}
}

func TestOpenclawBackend_Register_MergesExistingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".openclaw", "openclaw.json")
	writeJSON(t, configPath, map[string]any{"other": true})

	b := newOpenclawBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Action != port.ActionAdded {
		t.Errorf("Action = %v, want ActionAdded", res.Action)
	}
	root := readJSON(t, configPath)
	if _, ok := root["other"]; !ok {
		t.Error("other key should be preserved")
	}
}

func TestOpenclawBackend_Register_AlreadyRegistered(t *testing.T) {
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
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Action != port.ActionAlreadyRegistered {
		t.Errorf("Action = %v, want ActionAlreadyRegistered", res.Action)
	}
}

func TestOpenclawBackend_Unregister_RemovesEntry(t *testing.T) {
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
	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionRemoved {
		t.Errorf("Action = %v, want ActionRemoved", res.Action)
	}
}

func TestOpenclawBackend_Unregister_EntryNotPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".openclaw", "openclaw.json")
	writeJSON(t, configPath, map[string]any{"mcp": map[string]any{"servers": map[string]any{}}})

	b := newOpenclawBackend(ops)
	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionNotFound {
		t.Errorf("Action = %v, want ActionNotFound", res.Action)
	}
}

func TestOpenclawBackend_Unregister_FileNotExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b := newOpenclawBackend(newTestOps(dir))

	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionNotFound {
		t.Errorf("Action = %v, want ActionNotFound", res.Action)
	}
}

// ---- OpenClaw path resolution -----------------------------------------------

func TestOpenclawBackend_PathResolution_EnvVarTakesPrecedence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom", "openclaw.json")
	ops := withEnv(newTestOps(dir), map[string]string{
		"OPENCLAW_CONFIG_PATH": customPath,
	})

	b := newOpenclawBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.ConfigPath != customPath {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, customPath)
	}
}

func TestOpenclawBackend_PathResolution_StateDirEnvVar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")
	ops := withEnv(newTestOps(dir), map[string]string{
		"OPENCLAW_STATE_DIR": stateDir,
	})

	b := newOpenclawBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	wantPath := filepath.Join(stateDir, "openclaw.json")
	if res.ConfigPath != wantPath {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, wantPath)
	}
}

func TestOpenclawBackend_PathResolution_LegacyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// No env vars set; only legacy path exists.
	legacyPath := filepath.Join(dir, ".clawdbot", "clawdbot.json")
	writeJSON(t, legacyPath, map[string]any{})

	b := newOpenclawBackend(newTestOps(dir))
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.ConfigPath != legacyPath {
		t.Errorf("ConfigPath = %q, want legacy path %q", res.ConfigPath, legacyPath)
	}
}

// ---- Hermes backend ---------------------------------------------------------

func TestHermesBackend_Register_CreatesNewFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// getenv returns "" by default — HERMES_HOME not set.
	b := newHermesBackend(newTestOps(dir))

	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Action != port.ActionCreated {
		t.Errorf("Action = %v, want ActionCreated", res.Action)
	}
	configPath := filepath.Join(dir, ".hermes", "config.yaml")
	if res.ConfigPath != configPath {
		t.Errorf("ConfigPath = %q, want %q", res.ConfigPath, configPath)
	}
	root := readYAML(t, configPath)
	leaf := navJSON(t, root, []string{"mcp_servers"})
	if _, ok := leaf["go-apply"]; !ok {
		t.Error("go-apply not found in mcp_servers")
	}
}

func TestHermesBackend_Register_MergesExistingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".hermes", "config.yaml")
	writeYAML(t, configPath, map[string]any{"other": true})

	b := newHermesBackend(ops)
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Action != port.ActionAdded {
		t.Errorf("Action = %v, want ActionAdded", res.Action)
	}
	root := readYAML(t, configPath)
	if _, ok := root["other"]; !ok {
		t.Error("other key should be preserved")
	}
}

func TestHermesBackend_Register_AlreadyRegistered(t *testing.T) {
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
	res, err := b.Register("go-apply", testEntry)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.Action != port.ActionAlreadyRegistered {
		t.Errorf("Action = %v, want ActionAlreadyRegistered", res.Action)
	}
}

func TestHermesBackend_Unregister_RemovesEntry(t *testing.T) {
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
	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionRemoved {
		t.Errorf("Action = %v, want ActionRemoved", res.Action)
	}
	root := readYAML(t, configPath)
	leaf := navJSON(t, root, []string{"mcp_servers"})
	if _, ok := leaf["go-apply"]; ok {
		t.Error("go-apply should be absent after Unregister")
	}
}

func TestHermesBackend_Unregister_EntryNotPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ops := newTestOps(dir)
	configPath := filepath.Join(dir, ".hermes", "config.yaml")
	writeYAML(t, configPath, map[string]any{"mcp_servers": map[string]any{}})

	b := newHermesBackend(ops)
	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionNotFound {
		t.Errorf("Action = %v, want ActionNotFound", res.Action)
	}
}

func TestHermesBackend_Unregister_FileNotExist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b := newHermesBackend(newTestOps(dir))

	res, err := b.Unregister("go-apply")
	if err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if res.Action != port.ActionNotFound {
		t.Errorf("Action = %v, want ActionNotFound", res.Action)
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

func TestNewRegistrar_Claude(t *testing.T) {
	t.Parallel()
	r, err := NewRegistrar("claude")
	if err != nil {
		t.Fatalf("NewRegistrar(claude): %v", err)
	}
	if _, ok := r.(*claudeBackend); !ok {
		t.Errorf("expected *claudeBackend, got %T", r)
	}
}

func TestNewRegistrar_Openclaw(t *testing.T) {
	t.Parallel()
	r, err := NewRegistrar("openclaw")
	if err != nil {
		t.Fatalf("NewRegistrar(openclaw): %v", err)
	}
	if _, ok := r.(*openclawBackend); !ok {
		t.Errorf("expected *openclawBackend, got %T", r)
	}
}

func TestNewRegistrar_Hermes(t *testing.T) {
	t.Parallel()
	r, err := NewRegistrar("hermes")
	if err != nil {
		t.Fatalf("NewRegistrar(hermes): %v", err)
	}
	if _, ok := r.(*hermesBackend); !ok {
		t.Errorf("expected *hermesBackend, got %T", r)
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
