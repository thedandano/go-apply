package agentconfig

import (
	"encoding/json"
	"fmt"

	"github.com/thedandano/go-apply/internal/port"
	"gopkg.in/yaml.v3"
)

// MergeJSON inserts serverName+entry into JSON bytes at keyPath, preserving other content.
// Returns (merged bytes, alreadyRegistered, error).
// alreadyRegistered=true if serverName already exists with the same command+args.
func MergeJSON(existing []byte, keyPath []string, serverName string, entry port.MCPServerEntry) ([]byte, bool, error) {
	root, err := unmarshalJSONMap(existing)
	if err != nil {
		return nil, false, fmt.Errorf("MergeJSON: unmarshal: %w", err)
	}

	leaf, err := walkOrCreate(root, keyPath)
	if err != nil {
		return nil, false, fmt.Errorf("MergeJSON: %w", err)
	}

	if current, ok := leaf[serverName]; ok {
		if entryMatches(current, entry) {
			return existing, true, nil
		}
	}

	leaf[serverName] = map[string]any{
		"command": entry.Command,
		"args":    entry.Args,
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("MergeJSON: marshal: %w", err)
	}
	return out, false, nil
}

// MergeYAML inserts serverName+entry into YAML bytes at keyPath, preserving other content.
// Returns (merged bytes, alreadyRegistered, error).
func MergeYAML(existing []byte, keyPath []string, serverName string, entry port.MCPServerEntry) ([]byte, bool, error) {
	root, err := unmarshalYAMLMap(existing)
	if err != nil {
		return nil, false, fmt.Errorf("MergeYAML: unmarshal: %w", err)
	}

	leaf, err := walkOrCreate(root, keyPath)
	if err != nil {
		return nil, false, fmt.Errorf("MergeYAML: %w", err)
	}

	if current, ok := leaf[serverName]; ok {
		if entryMatches(current, entry) {
			return existing, true, nil
		}
	}

	leaf[serverName] = map[string]any{
		"command": entry.Command,
		"args":    entry.Args,
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, false, fmt.Errorf("MergeYAML: marshal: %w", err)
	}
	// yaml.Marshal always appends a trailing newline; nothing extra needed.
	return out, false, nil
}

// RemoveJSON removes serverName from JSON bytes at keyPath.
// Returns (updated bytes, wasPresent, error). wasPresent=false if entry didn't exist.
func RemoveJSON(existing []byte, keyPath []string, serverName string) ([]byte, bool, error) {
	if len(existing) == 0 {
		return existing, false, nil
	}

	root, err := unmarshalJSONMap(existing)
	if err != nil {
		return nil, false, fmt.Errorf("RemoveJSON: unmarshal: %w", err)
	}

	leaf, ok := walkExisting(root, keyPath)
	if !ok {
		return existing, false, nil
	}

	if _, exists := leaf[serverName]; !exists {
		return existing, false, nil
	}

	delete(leaf, serverName)

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("RemoveJSON: marshal: %w", err)
	}
	return out, true, nil
}

// RemoveYAML removes serverName from YAML bytes at keyPath.
// Returns (updated bytes, wasPresent, error). wasPresent=false if entry didn't exist.
func RemoveYAML(existing []byte, keyPath []string, serverName string) ([]byte, bool, error) {
	if len(existing) == 0 {
		return existing, false, nil
	}

	root, err := unmarshalYAMLMap(existing)
	if err != nil {
		return nil, false, fmt.Errorf("RemoveYAML: unmarshal: %w", err)
	}

	leaf, ok := walkExisting(root, keyPath)
	if !ok {
		return existing, false, nil
	}

	if _, exists := leaf[serverName]; !exists {
		return existing, false, nil
	}

	delete(leaf, serverName)

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, false, fmt.Errorf("RemoveYAML: marshal: %w", err)
	}
	return out, true, nil
}

// unmarshalJSONMap decodes existing into map[string]any, treating nil/empty as empty map.
func unmarshalJSONMap(existing []byte) (map[string]any, error) {
	if len(existing) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(existing, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return map[string]any{}, nil
	}
	return m, nil
}

// unmarshalYAMLMap decodes existing into map[string]any, treating nil/empty as empty map.
func unmarshalYAMLMap(existing []byte) (map[string]any, error) {
	if len(existing) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := yaml.Unmarshal(existing, &m); err != nil {
		return nil, err
	}
	if m == nil {
		return map[string]any{}, nil
	}
	return m, nil
}

// walkOrCreate traverses root along keyPath, creating intermediate maps as needed,
// and returns the leaf map. It returns an error if an intermediate key holds a non-map value.
func walkOrCreate(root map[string]any, keyPath []string) (map[string]any, error) {
	current := root
	for _, k := range keyPath {
		switch v := current[k].(type) {
		case map[string]any:
			current = v
		case nil:
			child := map[string]any{}
			current[k] = child
			current = child
		default:
			return nil, fmt.Errorf("key %q exists but is not an object (got %T)", k, current[k])
		}
	}
	return current, nil
}

// walkExisting traverses root along keyPath without creating intermediate maps.
// Returns (leaf, true) if all keys exist, or (nil, false) on the first missing key.
func walkExisting(root map[string]any, keyPath []string) (map[string]any, bool) {
	current := root
	for _, k := range keyPath {
		v, ok := current[k]
		if !ok {
			return nil, false
		}
		child, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		current = child
	}
	return current, true
}

// entryMatches reports whether raw (from json.Unmarshal or yaml.v3 Unmarshal into any)
// represents an MCPServerEntry with the same Command and Args as entry.
// Both json.Unmarshal and yaml.v3 decode strings as string and arrays as []any.
func entryMatches(raw any, entry port.MCPServerEntry) bool {
	m, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := m["command"].(string)
	if cmd != entry.Command {
		return false
	}
	rawArgs, _ := m["args"].([]any)
	if len(rawArgs) != len(entry.Args) {
		return false
	}
	for i, a := range rawArgs {
		s, ok := a.(string)
		if !ok || s != entry.Args[i] {
			return false
		}
	}
	return true
}
