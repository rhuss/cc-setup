package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PluginInfo holds metadata for a single discovered plugin.
type PluginInfo struct {
	Name        string // e.g. "rosa-rhoai"
	Marketplace string // e.g. "cc-rosa-rhoai-dev-marketplace"
	Version     string // e.g. "1.0.0"
	Description string // from .claude-plugin/plugin.json or hooks/hooks.json
	Key         string // "name@marketplace" (enabledPlugins key)
}

// DiscoverPlugins walks the plugin cache directory and returns metadata for
// each discovered plugin, picking the latest version directory.
func DiscoverPlugins() ([]PluginInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}
	cacheDir := filepath.Join(home, ".claude", "plugins", "cache")

	marketplaces, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin cache: %w", err)
	}

	var plugins []PluginInfo
	for _, mpEntry := range marketplaces {
		if !mpEntry.IsDir() {
			continue
		}
		marketplace := mpEntry.Name()
		mpPath := filepath.Join(cacheDir, marketplace)

		pluginDirs, err := os.ReadDir(mpPath)
		if err != nil {
			continue
		}

		for _, pEntry := range pluginDirs {
			if !pEntry.IsDir() {
				continue
			}
			pluginName := pEntry.Name()
			pluginPath := filepath.Join(mpPath, pluginName)

			version := latestVersion(pluginPath)
			if version == "" {
				continue
			}

			versionPath := filepath.Join(pluginPath, version)
			desc := readPluginDescription(versionPath)

			plugins = append(plugins, PluginInfo{
				Name:        pluginName,
				Marketplace: marketplace,
				Version:     version,
				Description: desc,
				Key:         pluginName + "@" + marketplace,
			})
		}
	}

	sortPlugins(plugins)
	return plugins, nil
}

// latestVersion returns the last non-orphaned directory entry (lexicographic)
// inside a plugin directory. Claude Code marks orphaned plugins with a
// ".orphaned_at" file in the version directory; these are skipped.
func latestVersion(pluginDir string) string {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return ""
	}
	var latest string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Skip orphaned versions
		orphanMarker := filepath.Join(pluginDir, e.Name(), ".orphaned_at")
		if _, err := os.Stat(orphanMarker); err == nil {
			continue
		}
		latest = e.Name()
	}
	return latest
}

// readPluginDescription tries to extract a description from plugin metadata.
// It checks .claude-plugin/plugin.json first, then plugin.json at root,
// then hooks/hooks.json, then hooks.json at root.
func readPluginDescription(versionPath string) string {
	// Try .claude-plugin/plugin.json
	if desc := readDescFromJSON(filepath.Join(versionPath, ".claude-plugin", "plugin.json")); desc != "" {
		return desc
	}
	// Try plugin.json at root
	if desc := readDescFromJSON(filepath.Join(versionPath, "plugin.json")); desc != "" {
		return desc
	}
	// Try hooks/hooks.json
	if desc := readDescFromJSON(filepath.Join(versionPath, "hooks", "hooks.json")); desc != "" {
		return desc
	}
	// Try hooks.json at root (legacy layout)
	if desc := readDescFromJSON(filepath.Join(versionPath, "hooks.json")); desc != "" {
		return desc
	}
	return ""
}

// readDescFromJSON reads a JSON file and extracts the "description" field.
func readDescFromJSON(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var obj struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return ""
	}
	return obj.Description
}

// PluginSettingsPath returns the settings.json path for the given scope.
//
//	"user"    -> ~/.claude/settings.json
//	"project" -> .claude/settings.json (relative to cwd)
func PluginSettingsPath(scope string) string {
	if scope == "user" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".claude", "settings.json")
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, ".claude", "settings.json")
}

// ReadEnabledPlugins reads the enabledPlugins map from settings.json for
// the given scope. Returns a map of plugin key to enabled state.
func ReadEnabledPlugins(scope string) map[string]bool {
	path := PluginSettingsPath(scope)
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]bool{}
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]bool{}
	}

	pluginsRaw, ok := raw["enabledPlugins"]
	if !ok {
		return map[string]bool{}
	}

	var enabled map[string]bool
	if err := json.Unmarshal(pluginsRaw, &enabled); err != nil {
		return map[string]bool{}
	}
	return enabled
}

// EffectiveEnabledPlugins computes the effective plugin state for a scope.
// User scope is the base; project scope overlays on top. The effective state
// for "user" scope is just the user settings. For "project" scope, the user
// settings serve as the base and the project settings override individual entries.
func EffectiveEnabledPlugins(scope string) map[string]bool {
	userEnabled := ReadEnabledPlugins("user")
	if scope == "user" {
		return userEnabled
	}
	// Project scope: start with user as base, overlay project overrides
	projectEnabled := ReadEnabledPlugins("project")
	effective := make(map[string]bool, len(userEnabled))
	for k, v := range userEnabled {
		effective[k] = v
	}
	for k, v := range projectEnabled {
		effective[k] = v
	}
	return effective
}

// MergePluginSources combines discovered plugins with enabled maps from all
// scopes. Plugins referenced in any enabledPlugins but not in the cache are
// added with version "?" and empty description. The result is sorted by Key.
func MergePluginSources(discovered []PluginInfo, enabledMaps ...map[string]bool) []PluginInfo {
	seen := make(map[string]bool, len(discovered))
	merged := make([]PluginInfo, len(discovered))
	copy(merged, discovered)

	for _, p := range discovered {
		seen[p.Key] = true
	}

	for _, enabled := range enabledMaps {
		for key := range enabled {
			if seen[key] {
				continue
			}
			seen[key] = true
			name, marketplace := parsePluginKey(key)
			merged = append(merged, PluginInfo{
				Name:        name,
				Marketplace: marketplace,
				Version:     "?",
				Key:         key,
			})
		}
	}

	sortPlugins(merged)
	return merged
}

// WriteEnabledPlugins writes the enabledPlugins map to the settings.json for
// the given scope, preserving all other keys.
//
// For project scope, only entries that differ from user scope are written
// (project settings are overrides, not a full copy). If no overrides remain,
// the enabledPlugins key is removed from the project file.
//
// Returns the written file path.
func WriteEnabledPlugins(scope string, effective map[string]bool) (string, error) {
	path := PluginSettingsPath(scope)

	// For project scope, compute the delta from user scope
	toWrite := effective
	if scope == "project" {
		userEnabled := ReadEnabledPlugins("user")
		delta := make(map[string]bool)
		for k, v := range effective {
			if userVal, exists := userEnabled[k]; !exists || userVal != v {
				delta[k] = v
			}
		}
		toWrite = delta
	}

	var data map[string]json.RawMessage
	if content, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(content, &data); err != nil {
			data = make(map[string]json.RawMessage)
		}
	} else {
		data = make(map[string]json.RawMessage)
	}

	if len(toWrite) == 0 {
		// No overrides needed; remove the key if it exists
		delete(data, "enabledPlugins")
	} else {
		pluginsJSON, err := json.Marshal(toWrite)
		if err != nil {
			return "", fmt.Errorf("marshalling enabledPlugins: %w", err)
		}
		data["enabledPlugins"] = pluginsJSON
	}

	// If file would be empty object and didn't exist before, skip writing
	if len(data) == 0 {
		// Nothing to write
		return path, nil
	}

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

// parsePluginKey splits "name@marketplace" into its two parts.
func parsePluginKey(key string) (name, marketplace string) {
	parts := strings.SplitN(key, "@", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return key, ""
}

// sortPlugins sorts a slice of PluginInfo by Key.
func sortPlugins(plugins []PluginInfo) {
	for i := 1; i < len(plugins); i++ {
		for j := i; j > 0 && plugins[j].Key < plugins[j-1].Key; j-- {
			plugins[j], plugins[j-1] = plugins[j-1], plugins[j]
		}
	}
}
