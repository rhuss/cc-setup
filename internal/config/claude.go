package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const mcpJSONName = ".mcp.json"

// ConfigPath returns the Claude config file path for a scope.
func ConfigPath(scope string) string {
	if scope == "user" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".claude.json")
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, mcpJSONName)
}

// ReadMcpServers reads the mcpServers dict from the Claude config for the given scope.
func ReadMcpServers(scope string) ServerMap {
	path := ConfigPath(scope)
	data, err := os.ReadFile(path)
	if err != nil {
		return ServerMap{}
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return ServerMap{}
	}

	serversRaw, ok := raw["mcpServers"]
	if !ok {
		return ServerMap{}
	}

	var servers ServerMap
	if err := json.Unmarshal(serversRaw, &servers); err != nil {
		return ServerMap{}
	}
	return servers
}

// WriteMcpServers merges servers into the Claude config file, removing deselected servers.
// It preserves any existing keys in the file that are not mcpServers.
func WriteMcpServers(scope string, servers ServerMap, toRemove []string) (string, error) {
	path := ConfigPath(scope)

	// Read existing file to preserve other keys
	var data map[string]json.RawMessage
	if content, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(content, &data); err != nil {
			data = make(map[string]json.RawMessage)
		}
	} else {
		data = make(map[string]json.RawMessage)
	}

	// Read existing mcpServers
	existing := ServerMap{}
	if raw, ok := data["mcpServers"]; ok {
		_ = json.Unmarshal(raw, &existing)
	}

	// Merge new servers
	for name, entry := range servers {
		existing[name] = entry
	}

	// Remove deselected servers
	for _, name := range toRemove {
		delete(existing, name)
	}

	// Marshal mcpServers back
	serversJSON, err := json.Marshal(existing)
	if err != nil {
		return "", fmt.Errorf("marshalling servers: %w", err)
	}
	data["mcpServers"] = serversJSON

	// Write the complete file, preserving all keys
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling config: %w", err)
	}
	output = append(output, '\n')

	if err := os.WriteFile(path, output, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}
