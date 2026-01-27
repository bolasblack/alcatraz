// includes.go implements config file includes support.
// See AGD-022 for design decisions.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	toml "github.com/pelletier/go-toml/v2"
)

// LoadWithIncludes loads config with includes support.
// It processes includes recursively, merging configs in the order they are specified.
func LoadWithIncludes(path string) (Config, error) {
	return loadWithIncludes(path, make(map[string]bool))
}

// loadWithIncludes is the internal recursive implementation.
func loadWithIncludes(path string, visited map[string]bool) (Config, error) {
	absPath, err := validateAndMarkVisited(path, visited)
	if err != nil {
		return Config{}, err
	}

	raw, err := readRawConfig(path)
	if err != nil {
		return Config{}, err
	}

	baseDir := filepath.Dir(absPath)
	mergedConfig, err := processIncludes(raw.Includes, baseDir, visited)
	if err != nil {
		return Config{}, err
	}

	currentConfig, err := rawToConfig(raw)
	if err != nil {
		return Config{}, fmt.Errorf("failed to convert config %s: %w", path, err)
	}

	if len(raw.Includes) > 0 {
		return mergeConfigs(mergedConfig, currentConfig), nil
	}
	return currentConfig, nil
}

// validateAndMarkVisited resolves path and checks for circular references.
func validateAndMarkVisited(path string, visited map[string]bool) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %s: %w", path, err)
	}
	if visited[absPath] {
		return "", fmt.Errorf("circular include detected: %s", path)
	}
	visited[absPath] = true
	return absPath, nil
}

// readRawConfig reads and parses a TOML config file.
func readRawConfig(path string) (RawConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RawConfig{}, err
	}
	var raw RawConfig
	if err := toml.Unmarshal(data, &raw); err != nil {
		return RawConfig{}, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return raw, nil
}

// processIncludes loads and merges all included configs.
func processIncludes(includes []string, baseDir string, visited map[string]bool) (Config, error) {
	var merged Config
	for _, pattern := range includes {
		resolved := pattern
		if !filepath.IsAbs(pattern) {
			resolved = filepath.Join(baseDir, pattern)
		}

		files, err := expandGlob(resolved)
		if err != nil {
			return Config{}, fmt.Errorf("failed to expand glob %s: %w", pattern, err)
		}

		for _, file := range files {
			cfg, err := loadWithIncludes(file, visited)
			if err != nil {
				return Config{}, fmt.Errorf("failed to load include %s: %w", file, err)
			}
			merged = mergeConfigs(merged, cfg)
		}
	}
	return merged, nil
}

// rawToConfig converts RawConfig to Config without applying defaults.
func rawToConfig(raw RawConfig) (Config, error) {
	// Convert raw envs to EnvValue
	envs := make(map[string]EnvValue)
	for key, val := range raw.Envs {
		env, err := parseEnvValue(val)
		if err != nil {
			return Config{}, fmt.Errorf("env %s: %w", key, err)
		}
		envs[key] = env
	}

	return Config{
		Image:     raw.Image,
		Workdir:   raw.Workdir,
		Runtime:   raw.Runtime,
		Commands:  raw.Commands,
		Mounts:    raw.Mounts,
		Resources: raw.Resources,
		Envs:      envs,
		Network:   raw.Network,
	}, nil
}

// parseEnvValue converts a raw value to EnvValue.
// Accepts string or map[string]any with value and override_on_enter fields.
func parseEnvValue(val any) (EnvValue, error) {
	switch v := val.(type) {
	case string:
		return EnvValue{Value: v, OverrideOnEnter: false}, nil
	case map[string]any:
		var env EnvValue
		if value, ok := v["value"].(string); ok {
			env.Value = value
		}
		if override, ok := v["override_on_enter"].(bool); ok {
			env.OverrideOnEnter = override
		}
		return env, nil
	default:
		return EnvValue{}, fmt.Errorf("invalid type: %T", val)
	}
}

// isGlobPattern checks if the pattern contains glob special characters.
func isGlobPattern(pattern string) bool {
	for _, c := range pattern {
		switch c {
		case '*', '?', '[':
			return true
		}
	}
	return false
}

// expandGlob expands a glob pattern and returns sorted matched files.
// For literal paths (no glob characters), returns error if file doesn't exist.
// For glob patterns, returns empty slice if no files match.
func expandGlob(pattern string) ([]string, error) {
	if !isGlobPattern(pattern) {
		// Literal path - must exist
		if _, err := os.Stat(pattern); err != nil {
			return nil, err
		}
		return []string{pattern}, nil
	}

	// Glob pattern - empty result is OK
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	// Sort for deterministic order
	sort.Strings(matches)
	return matches, nil
}

// mergeConfigs merges overlay config into base config.
// Objects: deep merge (recursive)
// Arrays: append (concatenate)
// Same key: overlay wins
func mergeConfigs(base, overlay Config) Config {
	// Mirror type ensures all Config fields are explicitly handled (AGD-015).
	// Adding a new field to Config will cause a compile error here.
	type configFields struct {
		Image     string
		Workdir   string
		Runtime   RuntimeType
		Commands  Commands
		Mounts    []string
		Resources Resources
		Envs      map[string]EnvValue
		Network   Network
	}
	_ = configFields(base)
	_ = configFields(overlay)

	result := base

	// Simple fields: overlay wins if non-empty
	if overlay.Image != "" {
		result.Image = overlay.Image
	}
	if overlay.Workdir != "" {
		result.Workdir = overlay.Workdir
	}
	if overlay.Runtime != "" {
		result.Runtime = overlay.Runtime
	}

	// Commands: deep merge
	if overlay.Commands.Up != "" {
		result.Commands.Up = overlay.Commands.Up
	}
	if overlay.Commands.Enter != "" {
		result.Commands.Enter = overlay.Commands.Enter
	}

	// Mounts: append (concatenate arrays)
	if len(overlay.Mounts) > 0 {
		result.Mounts = append(result.Mounts, overlay.Mounts...)
	}

	// Resources: deep merge
	if overlay.Resources.Memory != "" {
		result.Resources.Memory = overlay.Resources.Memory
	}
	if overlay.Resources.CPUs != 0 {
		result.Resources.CPUs = overlay.Resources.CPUs
	}

	// Envs: merge maps (overlay wins for same keys)
	if result.Envs == nil && len(overlay.Envs) > 0 {
		result.Envs = make(map[string]EnvValue)
	}
	for key, val := range overlay.Envs {
		result.Envs[key] = val
	}

	// Network: deep merge
	if len(overlay.Network.LANAccess) > 0 {
		result.Network.LANAccess = append(result.Network.LANAccess, overlay.Network.LANAccess...)
	}

	return result
}
