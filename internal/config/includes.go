// includes.go implements config file composition (extends/includes).
// See AGD-033 for extends/includes design, AGD-034 for command append.
package config

import (
	"fmt"
	"path/filepath"
	"sort"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// LoadWithIncludes loads config with extends/includes support.
// It processes extends and includes recursively, merging configs per AGD-033 priority rules.
func LoadWithIncludes(env *util.Env, path string) (Config, error) {
	return loadWithIncludes(env, path, make(map[string]bool))
}

// loadWithIncludes is the internal recursive implementation.
// Processing order (AGD-033):
//  1. Load and parse raw config
//  2. Process extends files (they become the base)
//  3. Convert current file to Config, merge: current overlays extends result
//  4. Process includes files (they overlay current)
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

	// Step 1: Process extends (current file wins over extended files)
	extendsResult, err := processExtends(env, raw.Extends, baseDir, visited)
	if err != nil {
		return Config{}, err
	}

	// Step 2: Convert current file
	currentConfig, err := rawToConfig(raw)
	if err != nil {
		return Config{}, fmt.Errorf("failed to convert config %s: %w", path, err)
	}

	// Step 3: Merge extends: current overlays extends result (current wins)
	if len(raw.Extends) > 0 {
		currentConfig = mergeConfigs(extendsResult, currentConfig)
	}

	// Step 4: Process includes (included files win over current)
	// Fold includes one-by-one onto currentConfig so each append sees
	// the accumulated result (not just other includes merged together).
	if len(raw.Includes) > 0 {
		includeConfigs, err := loadFileRefs(env, raw.Includes, baseDir, visited)
		if err != nil {
			return Config{}, err
		}
		for _, cfg := range includeConfigs {
			currentConfig = mergeConfigs(currentConfig, cfg)
		}
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
		return "", fmt.Errorf("circular reference detected: %s", path)
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

// processExtends loads and merges extends refs with first-entry-wins priority.
// Fold right-to-left: start from last, each earlier entry is overlay (wins).
func processExtends(env *util.Env, refs []string, baseDir string, visited map[string]bool) (Config, error) {
	configs, err := loadFileRefs(env, refs, baseDir, visited)
	if err != nil {
		return Config{}, err
	}
	var result Config
	for i := len(configs) - 1; i >= 0; i-- {
		result = mergeConfigs(result, configs[i])
	}
	return result, nil
}

// loadFileRefs loads all referenced configs, expanding globs and resolving recursively.
func loadFileRefs(env *util.Env, refs []string, baseDir string, visited map[string]bool) ([]Config, error) {
	var configs []Config
	for _, pattern := range refs {
		resolved := pattern
		if !filepath.IsAbs(pattern) {
			resolved = filepath.Join(baseDir, pattern)
		}

		files, err := expandGlob(env, resolved)
		if err != nil {
			return nil, fmt.Errorf("failed to expand glob %s: %w", pattern, err)
		}

		for _, file := range files {
			cfg, err := loadWithIncludes(env, file, visited)
			if err != nil {
				return nil, fmt.Errorf("failed to load referenced config %s: %w", file, err)
			}
			configs = append(configs, cfg)
		}
	}
	return configs, nil
}

// rawToConfig converts RawConfig to Config without applying defaults.
func rawToConfig(raw RawConfig) (Config, error) {
	// Mirror type ensures all RawConfig fields are explicitly handled (AGD-015).
	type rawConfigFields struct {
		Extends        []string
		Includes       []string
		Image          string
		Workdir        string
		WorkdirExclude []string
		Runtime        RuntimeType
		Commands       RawCommands
		Mounts         RawMountSlice
		Resources      Resources
		Envs           RawEnvValueMap
		Network        Network
		Caps           RawCaps
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

	// Convert raw caps to Caps
	caps, err := parseCaps(raw.Caps)
	if err != nil {
		return Config{}, err
	}

	// Convert raw commands to Commands
	cmdUp, err := parseCommandValue(raw.Commands.Up)
	if err != nil {
		return Config{}, fmt.Errorf("commands.up: %w", err)
	}
	cmdEnter, err := parseCommandValue(raw.Commands.Enter)
	if err != nil {
		return Config{}, fmt.Errorf("commands.enter: %w", err)
	}

	return Config{
		Image:          raw.Image,
		Workdir:        raw.Workdir,
		WorkdirExclude: raw.WorkdirExclude,
		Runtime:        raw.Runtime,
		Commands:       Commands{Up: cmdUp, Enter: cmdEnter},
		Mounts:         mounts,
		Resources:      raw.Resources,
		Envs:           envs,
		Network:        raw.Network,
		Caps:           caps,
	}, nil
}

// parseCommandValue converts a raw value to CommandValue.
// Accepts string or map[string]any with command and append fields.
func parseCommandValue(val any) (CommandValue, error) {
	if val == nil {
		return CommandValue{}, nil
	}
	switch v := val.(type) {
	case string:
		return CommandValue{Command: v}, nil
	case map[string]any:
		var cv CommandValue
		if cmd, ok := v["command"].(string); ok {
			cv.Command = cmd
		}
		if append, ok := v["append"].(bool); ok {
			cv.Append = append
		}
		return cv, nil
	default:
		return CommandValue{}, fmt.Errorf("expected string or object, got %T", val)
	}
}

// parseCaps converts raw caps value to Caps.
// Supports two modes:
//   - nil: returns empty Caps (defaults applied later in LoadConfig)
//   - array of strings: additive mode (Drop=["ALL"], Add=defaults+user)
//   - object with drop/add: full control mode (use as-is)
//
// See AGD-026 for design rationale.
func parseCaps(val any) (Caps, error) {
	if val == nil {
		// No caps field - return empty, defaults applied in LoadConfig
		return Caps{}, nil
	}

	switch v := val.(type) {
	case []any:
		// Array mode (additive): user specifies additional caps beyond defaults
		userCaps := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return Caps{}, fmt.Errorf("caps[%d]: expected string, got %T", i, item)
			}
			userCaps = append(userCaps, s)
		}
		// Additive mode: drop ALL, add defaults + user caps
		add := append([]string{}, DefaultCaps...)
		for _, cap := range userCaps {
			// Avoid duplicates with defaults
			isDuplicate := false
			for _, def := range DefaultCaps {
				if cap == def {
					isDuplicate = true
					break
				}
			}
			if !isDuplicate {
				add = append(add, cap)
			}
		}
		return Caps{
			Drop: DefaultCapsDrop(),
			Add:  add,
		}, nil

	case map[string]any:
		// Object mode (full control): user specifies exact drop/add lists
		var caps Caps
		if drop, ok := v["drop"].([]any); ok {
			for i, item := range drop {
				s, ok := item.(string)
				if !ok {
					return Caps{}, fmt.Errorf("caps.drop[%d]: expected string, got %T", i, item)
				}
				caps.Drop = append(caps.Drop, s)
			}
		}
		if add, ok := v["add"].([]any); ok {
			for i, item := range add {
				s, ok := item.(string)
				if !ok {
					return Caps{}, fmt.Errorf("caps.add[%d]: expected string, got %T", i, item)
				}
				caps.Add = append(caps.Add, s)
			}
		}
		return caps, nil

	default:
		return Caps{}, fmt.Errorf("caps: expected array or object, got %T", val)
	}
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
		Caps           Caps
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

	// Commands: deep merge with append support (AGD-033)
	result.Commands.Up = mergeCommandValue(base.Commands.Up, overlay.Commands.Up)
	result.Commands.Enter = mergeCommandValue(base.Commands.Enter, overlay.Commands.Enter)

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

	// Caps: overlay wins if non-empty (full replacement, not merge)
	if len(overlay.Caps.Drop) > 0 || len(overlay.Caps.Add) > 0 {
		result.Caps = overlay.Caps
	}

	return result
}

// mergeCommandValue merges two CommandValues with append support.
// If overlay is empty, base is returned unchanged.
// If overlay has Append=true and base is non-empty, commands are space-concatenated.
// Otherwise overlay replaces base.
func mergeCommandValue(base, overlay CommandValue) CommandValue {
	if overlay.Command == "" {
		return base
	}
	if overlay.Append && base.Command != "" {
		return CommandValue{
			Command: base.Command + " " + overlay.Command,
			Append:  false, // append is consumed during merge
		}
	}
	return CommandValue{
		Command: overlay.Command,
		Append:  overlay.Append, // preserve for later merges in layered resolution
	}
}
