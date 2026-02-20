package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

const settingsFileName = "settings.local.json"

// SettingsPath returns the path to settings.local.json for the given scope.
// "user" -> ~/.claude/settings.local.json
// "project" -> .claude/settings.local.json (relative to cwd)
func SettingsPath(scope string) string {
	if scope == "user" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".claude", settingsFileName)
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, ".claude", settingsFileName)
}

// mcpPermPrefix returns the permission entry prefix for a server,
// e.g. "mcp__my-server__".
func mcpPermPrefix(serverName string) string {
	return "mcp__" + serverName + "__"
}

// ReadToolPermissions reads settings.local.json for the given scope and returns
// the list of tool names that have permissions.allow entries matching
// mcp__<serverName>__*. Returns ["*"] if a wildcard entry exists.
func ReadToolPermissions(scope, serverName string) []string {
	path := SettingsPath(scope)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	permsRaw, ok := raw["permissions"]
	if !ok {
		return nil
	}

	var perms struct {
		Allow []string `json:"allow"`
	}
	if err := json.Unmarshal(permsRaw, &perms); err != nil {
		return nil
	}

	prefix := mcpPermPrefix(serverName)
	wildcard := prefix + "*"
	var tools []string
	for _, entry := range perms.Allow {
		if entry == wildcard {
			return []string{"*"}
		}
		if strings.HasPrefix(entry, prefix) {
			toolName := strings.TrimPrefix(entry, prefix)
			if toolName != "" {
				tools = append(tools, toolName)
			}
		}
	}
	return tools
}

// WriteToolPermissions updates the permissions.allow array in settings.local.json
// for the given scope. It removes all existing mcp__<serverName>__* entries, then
// adds new entries based on toolNames. If all tools are selected (len(toolNames)
// == len(allToolNames)), a single wildcard entry is written instead.
// Deselected tools are also removed from permissions.deny to avoid conflicts.
// Returns the written file path.
func WriteToolPermissions(scope, serverName string, toolNames []string, allToolNames []string) (string, error) {
	path := SettingsPath(scope)

	// Read existing file to preserve all other keys
	var data map[string]json.RawMessage
	if content, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(content, &data); err != nil {
			data = make(map[string]json.RawMessage)
		}
	} else {
		data = make(map[string]json.RawMessage)
	}

	// Parse existing permissions
	type permissionsBlock struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	var perms permissionsBlock
	if raw, ok := data["permissions"]; ok {
		_ = json.Unmarshal(raw, &perms)
	}

	prefix := mcpPermPrefix(serverName)

	// Remove all existing entries for this server from allow
	filtered := make([]string, 0, len(perms.Allow))
	for _, entry := range perms.Allow {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}

	// Add new entries
	if len(toolNames) > 0 {
		if len(toolNames) == len(allToolNames) {
			// All selected: use wildcard
			filtered = append(filtered, prefix+"*")
		} else {
			for _, t := range toolNames {
				filtered = append(filtered, prefix+t)
			}
		}
	}
	perms.Allow = filtered

	// Build a set of selected tools for deny cleanup
	selectedSet := make(map[string]bool, len(toolNames))
	for _, t := range toolNames {
		selectedSet[t] = true
	}

	// Remove deselected tools from deny to avoid conflicts
	filteredDeny := make([]string, 0, len(perms.Deny))
	for _, entry := range perms.Deny {
		if strings.HasPrefix(entry, prefix) {
			toolName := strings.TrimPrefix(entry, prefix)
			// Keep deny entries for tools that are NOT in the selected set
			// (i.e., only remove deny entries for tools we're allowing)
			if selectedSet[toolName] {
				continue
			}
		}
		filteredDeny = append(filteredDeny, entry)
	}
	perms.Deny = filteredDeny

	// Marshal permissions back
	permsJSON, err := json.Marshal(perms)
	if err != nil {
		return "", fmt.Errorf("marshalling permissions: %w", err)
	}
	data["permissions"] = permsJSON

	// Write the file, creating parent directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating directory %s: %w", dir, err)
	}

	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshalling settings: %w", err)
	}
	output = append(output, '\n')

	if err := os.WriteFile(path, output, 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}
