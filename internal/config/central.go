package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ServerMap maps server names to their full definitions.
// Using map[string]any preserves unknown fields and varying shapes
// (http vs stdio, with/without headers, env vars, etc.).
type ServerMap map[string]map[string]any

// centralConfig is the on-disk format of mcp.json.
type centralConfig struct {
	Servers ServerMap `json:"servers"`
}

// GetConfigDir returns the config directory, respecting XDG_CONFIG_HOME.
func GetConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mcp-setup")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "mcp-setup")
}

// GetConfigFile returns the path to the central server config.
func GetConfigFile() string {
	return filepath.Join(GetConfigDir(), "mcp.json")
}

// LoadServers reads server definitions from the central config.
func LoadServers() (ServerMap, error) {
	path := GetConfigFile()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ServerMap{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var cfg centralConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if cfg.Servers == nil {
		return ServerMap{}, nil
	}
	return cfg.Servers, nil
}

// SaveServers writes server definitions to the central config.
func SaveServers(servers ServerMap) error {
	path := GetConfigFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	cfg := centralConfig{Servers: servers}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0o644)
}

// ServerNames returns sorted server names from a ServerMap.
func ServerNames(servers ServerMap) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	// Sort for stable output
	sortStrings(names)
	return names
}

// sortStrings sorts a slice of strings in place.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// BuildEntryForClaude strips the description field from a server definition
// so it can be written to Claude's config.
func BuildEntryForClaude(serverDef map[string]any) map[string]any {
	entry := make(map[string]any, len(serverDef))
	for k, v := range serverDef {
		if k != "description" {
			entry[k] = v
		}
	}
	return entry
}
