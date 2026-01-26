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
	// Get absolute path for circular reference detection
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("failed to resolve path %s: %w", path, err)
	}

	// Check for circular reference
	if visited[absPath] {
		return Config{}, fmt.Errorf("circular include detected: %s", path)
	}
	visited[absPath] = true

	// Read and parse raw config
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var raw rawConfig
	if err := toml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	// Get base directory for resolving relative include paths
	baseDir := filepath.Dir(absPath)

	// Process includes first (depth-first)
	var mergedConfig Config
	hasIncludes := len(raw.Includes) > 0

	for _, includePattern := range raw.Includes {
		// Resolve relative to the current config file's directory
		resolvedPattern := includePattern
		if !filepath.IsAbs(includePattern) {
			resolvedPattern = filepath.Join(baseDir, includePattern)
		}

		// Expand glob pattern
		matchedFiles, err := expandGlob(resolvedPattern)
		if err != nil {
			return Config{}, fmt.Errorf("failed to expand glob %s: %w", includePattern, err)
		}

		// Empty glob result is OK (no files matched)
		for _, includePath := range matchedFiles {
			// Recursively load included config
			includedConfig, err := loadWithIncludes(includePath, visited)
			if err != nil {
				return Config{}, fmt.Errorf("failed to load include %s: %w", includePath, err)
			}

			// Merge included config
			mergedConfig = mergeConfigs(mergedConfig, includedConfig)
		}
	}

	// Convert current raw config to Config
	currentConfig, err := rawToConfig(raw)
	if err != nil {
		return Config{}, fmt.Errorf("failed to convert config %s: %w", path, err)
	}

	// Merge current config on top of included configs
	if hasIncludes {
		mergedConfig = mergeConfigs(mergedConfig, currentConfig)
	} else {
		mergedConfig = currentConfig
	}

	return mergedConfig, nil
}

// rawToConfig converts rawConfig to Config without applying defaults.
func rawToConfig(raw rawConfig) (Config, error) {
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
	}, nil
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

	return result
}
