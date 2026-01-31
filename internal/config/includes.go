// includes.go implements config file includes support.
// See AGD-022 for design decisions.
package config

import (
	"fmt"
	"path/filepath"
	"sort"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// LoadWithIncludes loads config with includes support.
// It processes includes recursively, merging configs in the order they are specified.
func LoadWithIncludes(env *util.Env, path string) (Config, error) {
	return loadWithIncludes(env, path, make(map[string]bool))
}

// loadWithIncludes is the internal recursive implementation.
func loadWithIncludes(env *util.Env, path string, visited map[string]bool) (Config, error) {
	absPath, err := validateAndMarkVisited(path, visited)
	if err != nil {
		return Config{}, err
	}

	raw, err := readRawConfig(env, path)
	if err != nil {
		return Config{}, err
	}

	baseDir := filepath.Dir(absPath)
	mergedConfig, err := processIncludes(env, raw.Includes, baseDir, visited)
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
func readRawConfig(env *util.Env, path string) (RawConfig, error) {
	data, err := afero.ReadFile(env.Fs, path)
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
func processIncludes(env *util.Env, includes []string, baseDir string, visited map[string]bool) (Config, error) {
	var merged Config
	for _, pattern := range includes {
		resolved := pattern
		if !filepath.IsAbs(pattern) {
			resolved = filepath.Join(baseDir, pattern)
		}

		files, err := expandGlob(env, resolved)
		if err != nil {
			return Config{}, fmt.Errorf("failed to expand glob %s: %w", pattern, err)
		}

		for _, file := range files {
			cfg, err := loadWithIncludes(env, file, visited)
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
	// Mirror type ensures all RawConfig fields are explicitly handled (AGD-015).
	type rawConfigFields struct {
		Includes       []string
		Image          string
		Workdir        string
		WorkdirExclude []string
		Runtime        RuntimeType
		Commands       Commands
		Mounts         RawMountSlice
		Resources      Resources
		Envs           RawEnvValueMap
		Network        Network
	}
	_ = rawConfigFields(raw)

	// Convert raw envs to EnvValue
	envs := make(map[string]EnvValue)
	for key, val := range raw.Envs {
		env, err := parseEnvValue(val)
		if err != nil {
			return Config{}, fmt.Errorf("env %s: %w", key, err)
		}
		envs[key] = env
	}

	// Convert raw mounts to MountConfig
	mounts, err := parseMounts(raw.Mounts)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Image:          raw.Image,
		Workdir:        raw.Workdir,
		WorkdirExclude: raw.WorkdirExclude,
		Runtime:        raw.Runtime,
		Commands:       raw.Commands,
		Mounts:         mounts,
		Resources:      raw.Resources,
		Envs:           envs,
		Network:        raw.Network,
	}, nil
}

// parseMounts converts raw mount values to MountConfig slice.
// Accepts both string format ("source:target[:ro]") and object format.
func parseMounts(raw []any) ([]MountConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	mounts := make([]MountConfig, 0, len(raw))
	for i, val := range raw {
		m, err := parseMountValue(val)
		if err != nil {
			return nil, fmt.Errorf("mount[%d]: %w", i, err)
		}
		mounts = append(mounts, m)
	}
	return mounts, nil
}

// parseMountValue converts a single raw mount value to MountConfig.
func parseMountValue(val any) (MountConfig, error) {
	switch v := val.(type) {
	case string:
		return ParseMount(v)
	case map[string]any:
		return parseMountObject(v)
	default:
		return MountConfig{}, fmt.Errorf("invalid type: %T", val)
	}
}

// parseMountObject parses a mount object with source, target, readonly, exclude fields.
func parseMountObject(m map[string]any) (MountConfig, error) {
	var mc MountConfig

	source, ok := m["source"].(string)
	if !ok || source == "" {
		return MountConfig{}, fmt.Errorf("mount source is required")
	}
	mc.Source = source

	target, ok := m["target"].(string)
	if !ok || target == "" {
		return MountConfig{}, fmt.Errorf("mount target is required")
	}
	mc.Target = target

	if readonly, ok := m["readonly"].(bool); ok {
		mc.Readonly = readonly
	}

	if exclude, ok := m["exclude"].([]any); ok {
		for i, e := range exclude {
			s, ok := e.(string)
			if !ok {
				return MountConfig{}, fmt.Errorf("exclude[%d]: expected string, got %T", i, e)
			}
			mc.Exclude = append(mc.Exclude, s)
		}
	}

	return mc, nil
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
func expandGlob(env *util.Env, pattern string) ([]string, error) {
	if !isGlobPattern(pattern) {
		// Literal path - must exist
		if _, err := env.Fs.Stat(pattern); err != nil {
			return nil, err
		}
		return []string{pattern}, nil
	}

	// Glob pattern - empty result is OK
	matches, err := afero.Glob(env.Fs, pattern)
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
		Image          string
		Workdir        string
		WorkdirExclude []string
		Runtime        RuntimeType
		Commands       Commands
		Mounts         []MountConfig
		Resources      Resources
		Envs           map[string]EnvValue
		Network        Network
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
	if len(overlay.WorkdirExclude) > 0 {
		result.WorkdirExclude = overlay.WorkdirExclude
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
