package agentconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/thedandano/go-apply/internal/port"
)

// fileOps bundles all filesystem and environment operations so they can be
// injected by tests without requiring real OS state.
type fileOps struct {
	readFile   func(string) ([]byte, error)
	writeFile  func(string, []byte, fs.FileMode) error
	stat       func(string) (fs.FileInfo, error)
	mkdirAll   func(string, fs.FileMode) error
	removeAll  func(string) error     // os.RemoveAll in production
	executable func() (string, error) // os.Executable in production
	getenv     func(string) string
	homeDir    string
	goos       string // runtime.GOOS in production; overrideable in tests
}

// newProdFileOps builds a fileOps using real OS functions. It resolves
// homeDir eagerly so callers get an error at construction time rather than
// at first use.
func newProdFileOps() (*fileOps, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory: %w", err)
	}
	return &fileOps{
		readFile:   os.ReadFile,
		writeFile:  os.WriteFile,
		stat:       os.Stat,
		mkdirAll:   os.MkdirAll,
		removeAll:  os.RemoveAll,
		executable: os.Executable,
		getenv:     os.Getenv,
		homeDir:    home,
		goos:       runtime.GOOS,
	}, nil
}

// exists returns true when the path stat-check succeeds.
func (f *fileOps) exists(path string) bool {
	_, err := f.stat(path)
	return err == nil
}

// ---- Per-agent backend structs ----------------------------------------------

type claudeBackend struct{ ops *fileOps }
type openclawBackend struct{ ops *fileOps }
type hermesBackend struct{ ops *fileOps }

// Compile-time interface assertions.
var _ port.AgentConfigRegistrar = (*claudeBackend)(nil)
var _ port.AgentConfigRegistrar = (*openclawBackend)(nil)
var _ port.AgentConfigRegistrar = (*hermesBackend)(nil)

// ---- Unexported constructors (used by tests) --------------------------------

func newClaudeBackend(ops *fileOps) *claudeBackend     { return &claudeBackend{ops: ops} }
func newOpenclawBackend(ops *fileOps) *openclawBackend { return &openclawBackend{ops: ops} }
func newHermesBackend(ops *fileOps) *hermesBackend     { return &hermesBackend{ops: ops} }

// ---- Path resolution --------------------------------------------------------

// configPath returns the Claude config file path:
//  1. ~/.claude/settings.json if it exists
//  2. ~/Library/Application Support/Claude/claude_desktop_config.json (macOS only, if exists)
//  3. ~/.claude/settings.json as default
func (b *claudeBackend) configPath() string {
	cliPath := filepath.Join(b.ops.homeDir, ".claude", "settings.json")
	if b.ops.exists(cliPath) {
		return cliPath
	}
	if b.ops.goos == "darwin" {
		desktopPath := filepath.Join(b.ops.homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
		if b.ops.exists(desktopPath) {
			return desktopPath
		}
	}
	return cliPath
}

// configPath returns the OpenClaw config file path, checking in priority order:
//  1. $OPENCLAW_CONFIG_PATH env var
//  2. $OPENCLAW_STATE_DIR/openclaw.json
//  3. ~/.openclaw/openclaw.json (modern, if exists)
//  4. ~/.clawdbot/clawdbot.json (legacy, if exists)
//  5. ~/.openclaw/openclaw.json as default
func (b *openclawBackend) configPath() string {
	if v := b.ops.getenv("OPENCLAW_CONFIG_PATH"); v != "" {
		return v
	}
	if v := b.ops.getenv("OPENCLAW_STATE_DIR"); v != "" {
		return filepath.Join(v, "openclaw.json")
	}
	modernPath := filepath.Join(b.ops.homeDir, ".openclaw", "openclaw.json")
	if b.ops.exists(modernPath) {
		return modernPath
	}
	legacyPath := filepath.Join(b.ops.homeDir, ".clawdbot", "clawdbot.json")
	if b.ops.exists(legacyPath) {
		return legacyPath
	}
	return modernPath
}

// configPath returns the Hermes config file path:
//  1. $HERMES_HOME/config.yaml if HERMES_HOME is set
//  2. ~/.hermes/config.yaml as default
func (b *hermesBackend) configPath() string {
	if v := b.ops.getenv("HERMES_HOME"); v != "" {
		return filepath.Join(v, "config.yaml")
	}
	return filepath.Join(b.ops.homeDir, ".hermes", "config.yaml")
}

// ---- Shared register/unregister helpers ------------------------------------

type mergeFunc func([]byte, []string, string, port.MCPServerEntry) ([]byte, bool, error)
type removeFunc func([]byte, []string, string) ([]byte, bool, error)

// registerWith performs the shared register flow for any backend.
// When force is true, the "already registered" short-circuit is skipped and the
// entry is written unconditionally.
func registerWith(ops *fileOps, path string, keyPath []string, serverName string, entry port.MCPServerEntry, merge mergeFunc, force bool) (port.RegistrationResult, error) {
	existing, err := ops.readFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return port.RegistrationResult{}, fmt.Errorf("read %s: %w", path, err)
	}
	isNew := errors.Is(err, fs.ErrNotExist)

	merged, already, err := merge(existing, keyPath, serverName, entry)
	if err != nil {
		return port.RegistrationResult{}, err
	}
	if already && !force {
		return port.RegistrationResult{ConfigPath: path, Action: port.ActionAlreadyRegistered}, nil
	}

	if err := ops.mkdirAll(filepath.Dir(path), 0o700); err != nil {
		return port.RegistrationResult{}, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := ops.writeFile(path, merged, 0o600); err != nil {
		return port.RegistrationResult{}, fmt.Errorf("write %s: %w", path, err)
	}

	action := port.ActionAdded
	if isNew {
		action = port.ActionCreated
	}
	return port.RegistrationResult{ConfigPath: path, Action: action}, nil
}

// unregisterWith performs the shared unregister flow for any backend.
func unregisterWith(ops *fileOps, path string, keyPath []string, serverName string, remove removeFunc) (port.RegistrationResult, error) {
	existing, err := ops.readFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return port.RegistrationResult{ConfigPath: path, Action: port.ActionNotFound}, nil
	}
	if err != nil {
		return port.RegistrationResult{}, fmt.Errorf("read %s: %w", path, err)
	}

	updated, wasPresent, err := remove(existing, keyPath, serverName)
	if err != nil {
		return port.RegistrationResult{}, err
	}
	if !wasPresent {
		return port.RegistrationResult{ConfigPath: path, Action: port.ActionNotFound}, nil
	}

	if err := ops.writeFile(path, updated, 0o600); err != nil {
		return port.RegistrationResult{}, fmt.Errorf("write %s: %w", path, err)
	}
	return port.RegistrationResult{ConfigPath: path, Action: port.ActionRemoved}, nil
}

// ---- claudeBackend implementation ------------------------------------------

const claudePluginsDir = ".claude/plugins"

func (b *claudeBackend) pluginDir(serverName string) string {
	return filepath.Join(b.ops.homeDir, claudePluginsDir, serverName)
}

// writeClaudePlugin writes the plugin.json and .mcp.json files for the given server.
func (b *claudeBackend) writeClaudePlugin(serverName, pluginJSONPath, mcpJSONPath string) (string, error) {
	pluginDir := b.pluginDir(serverName)

	binPath, err := b.ops.executable()
	if err != nil {
		binPath = "go-apply"
	}

	// Create .claude-plugin/ directory.
	if err := b.ops.mkdirAll(filepath.Dir(pluginJSONPath), 0o700); err != nil {
		return "", fmt.Errorf("mkdir plugin dir: %w", err)
	}

	pluginContent, _ := json.MarshalIndent(map[string]any{
		"name":        serverName,
		"description": "Score resumes against job postings, tailor resumes, and generate cover letters",
		"author":      map[string]any{"name": "Dan Sedano"},
	}, "", "  ")
	if err := b.ops.writeFile(pluginJSONPath, pluginContent, 0o600); err != nil {
		return "", fmt.Errorf("write plugin.json: %w", err)
	}

	mcpContent, _ := json.MarshalIndent(map[string]any{
		serverName: map[string]any{
			"command": binPath,
			"args":    []string{"serve"},
		},
	}, "", "  ")
	if err := b.ops.writeFile(mcpJSONPath, mcpContent, 0o600); err != nil {
		return "", fmt.Errorf("write .mcp.json: %w", err)
	}

	return pluginDir, nil
}

func (b *claudeBackend) Register(serverName string, _ port.MCPServerEntry) (port.RegistrationResult, error) {
	pluginDir := b.pluginDir(serverName)
	mcpJSONPath := filepath.Join(pluginDir, ".mcp.json")
	pluginJSONPath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")

	if b.ops.exists(mcpJSONPath) {
		return port.RegistrationResult{ConfigPath: pluginDir, Action: port.ActionAlreadyRegistered}, nil
	}

	dir, err := b.writeClaudePlugin(serverName, pluginJSONPath, mcpJSONPath)
	if err != nil {
		return port.RegistrationResult{}, err
	}
	return port.RegistrationResult{ConfigPath: dir, Action: port.ActionCreated}, nil
}

// RegisterForce overwrites an existing Claude plugin registration unconditionally.
func (b *claudeBackend) RegisterForce(serverName string, _ port.MCPServerEntry) (port.RegistrationResult, error) {
	pluginDir := b.pluginDir(serverName)
	mcpJSONPath := filepath.Join(pluginDir, ".mcp.json")
	pluginJSONPath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")

	dir, err := b.writeClaudePlugin(serverName, pluginJSONPath, mcpJSONPath)
	if err != nil {
		return port.RegistrationResult{}, err
	}
	return port.RegistrationResult{ConfigPath: dir, Action: port.ActionCreated}, nil
}

func (b *claudeBackend) Unregister(serverName string) (port.RegistrationResult, error) {
	pluginDir := b.pluginDir(serverName)

	pluginRemoved := false
	if b.ops.exists(pluginDir) {
		if err := b.ops.removeAll(pluginDir); err != nil {
			return port.RegistrationResult{}, fmt.Errorf("remove plugin dir: %w", err)
		}
		pluginRemoved = true
	}

	// Clean up any stale mcpServers entry in settings.json.
	staleResult, err := unregisterWith(b.ops, b.configPath(), []string{"mcpServers"}, serverName, RemoveJSON)
	if err != nil {
		return port.RegistrationResult{}, err
	}
	_ = staleResult // used only for side-effect cleanup

	if !pluginRemoved {
		return port.RegistrationResult{ConfigPath: pluginDir, Action: port.ActionNotFound}, nil
	}
	return port.RegistrationResult{ConfigPath: pluginDir, Action: port.ActionRemoved}, nil
}

// ---- openclawBackend implementation ----------------------------------------

func (b *openclawBackend) Register(serverName string, entry port.MCPServerEntry) (port.RegistrationResult, error) {
	return registerWith(b.ops, b.configPath(), []string{"mcp", "servers"}, serverName, entry, MergeJSON, false)
}

// RegisterForce overwrites an existing openclaw registration unconditionally.
func (b *openclawBackend) RegisterForce(serverName string, entry port.MCPServerEntry) (port.RegistrationResult, error) {
	return registerWith(b.ops, b.configPath(), []string{"mcp", "servers"}, serverName, entry, MergeJSON, true)
}

func (b *openclawBackend) Unregister(serverName string) (port.RegistrationResult, error) {
	return unregisterWith(b.ops, b.configPath(), []string{"mcp", "servers"}, serverName, RemoveJSON)
}

// ---- hermesBackend implementation ------------------------------------------

func (b *hermesBackend) Register(serverName string, entry port.MCPServerEntry) (port.RegistrationResult, error) {
	return registerWith(b.ops, b.configPath(), []string{"mcp_servers"}, serverName, entry, MergeYAML, false)
}

// RegisterForce overwrites an existing hermes registration unconditionally.
func (b *hermesBackend) RegisterForce(serverName string, entry port.MCPServerEntry) (port.RegistrationResult, error) {
	return registerWith(b.ops, b.configPath(), []string{"mcp_servers"}, serverName, entry, MergeYAML, true)
}

func (b *hermesBackend) Unregister(serverName string) (port.RegistrationResult, error) {
	return unregisterWith(b.ops, b.configPath(), []string{"mcp_servers"}, serverName, RemoveYAML)
}

// ---- Registry factory -------------------------------------------------------

// NewRegistrar returns an AgentConfigRegistrar for the given agent name.
// Valid names: "claude", "openclaw", "hermes".
func NewRegistrar(agentName string) (port.AgentConfigRegistrar, error) {
	ops, err := newProdFileOps()
	if err != nil {
		return nil, err
	}
	switch agentName {
	case "claude":
		return newClaudeBackend(ops), nil
	case "openclaw":
		return newOpenclawBackend(ops), nil
	case "hermes":
		return newHermesBackend(ops), nil
	default:
		return nil, fmt.Errorf("unknown agent %q: valid agents are claude, openclaw, hermes", agentName)
	}
}
