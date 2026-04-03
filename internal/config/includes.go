// includes.go implements config file composition (extends/includes).
// See AGD-033 for extends/includes design, AGD-034 for command append.
package config

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// LoadWithIncludes loads config with extends/includes support.
// It processes extends and includes recursively, merging configs per AGD-033 priority rules.
// expandEnv expands ${VAR} references in include/extend paths (use os.ExpandEnv for production).
func LoadWithIncludes(env *util.Env, path string, expandEnv func(string) (string, error)) (Config, error) {
	return loadWithIncludes(env, path, expandEnv, make(map[string]bool))
}

// loadWithIncludes is the internal recursive implementation.
// Processing order (AGD-033):
//  1. Load and parse raw config
//  2. Process extends files (they become the base)
//  3. Convert current file to Config, merge: current overlays extends result
//  4. Process includes files (they overlay current)
func loadWithIncludes(env *util.Env, path string, expandEnv func(string) (string, error), visited map[string]bool) (Config, error) {
	absPath, err := validateAndMarkVisited(path, visited)
	if err != nil {
		return Config{}, err
	}

	raw, err := readRawConfig(env, path)
	if err != nil {
		return Config{}, err
	}

	// Step 1: Process extends (current file wins over extended files)
	extendsResult, err := processExtends(env, raw.Extends, absPath, expandEnv, visited)
	if err != nil {
		return Config{}, err
	}

	// Step 2: Convert current file
	currentConfig, err := rawToConfig(raw, expandEnv)
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
		includeConfigs, err := loadFileRefs(env, raw.Includes, absPath, expandEnv, visited)
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
		return "", fmt.Errorf("circular reference detected: %s: %w", path, ErrCircularReference)
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
func processExtends(env *util.Env, refs []string, configFilePath string, expandEnv func(string) (string, error), visited map[string]bool) (Config, error) {
	configs, err := loadFileRefs(env, refs, configFilePath, expandEnv, visited)
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
func loadFileRefs(env *util.Env, refs []string, configFilePath string, expandEnv func(string) (string, error), visited map[string]bool) ([]Config, error) {
	var configs []Config
	for _, rawPath := range refs {
		ref := NewConfigFileRef(configFilePath, rawPath)
		files, err := ref.Expand(expandEnv, env.Fs)
		if err != nil {
			return nil, fmt.Errorf("failed to expand ref %s: %w", rawPath, err)
		}

		for _, file := range files {
			cfg, err := loadWithIncludes(env, file, expandEnv, visited)
			if err != nil {
				return nil, fmt.Errorf("failed to load referenced config %s: %w", file, err)
			}
			configs = append(configs, cfg)
		}
	}
	return configs, nil
}

// rawToConfig converts RawConfig to Config without applying defaults.
// expandEnv expands ${VAR} references in mount source paths (not target).
func rawToConfig(raw RawConfig, expandEnv func(string) (string, error)) (Config, error) {
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
		Network        RawNetwork
		Caps           RawCaps
	}
	// Verify: if a field is added to RawConfig but not here, this line fails to compile.
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
	mounts, err := parseMounts(raw.Mounts, expandEnv)
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

	// Convert raw ports to PortConfig
	ports, err := parsePorts(raw.Network.Ports)
	if err != nil {
		return Config{}, err
	}

	// Mirror type ensures all RawNetwork fields are explicitly handled (AGD-015).
	type rawNetworkFields struct {
		LANAccess []string
		Ports     RawPortSlice
		Proxy     string
	}
	_ = rawNetworkFields(raw.Network)

	// Mirror type ensures all Network fields are explicitly handled (AGD-015).
	type networkFields struct {
		LANAccess []string
		Ports     []PortConfig
		Proxy     string
	}
	network := Network{
		LANAccess: raw.Network.LANAccess,
		Ports:     ports,
		Proxy:     raw.Network.Proxy,
	}
	_ = networkFields(network)

	return Config{
		Image:          raw.Image,
		Workdir:        raw.Workdir,
		WorkdirExclude: raw.WorkdirExclude,
		Runtime:        raw.Runtime,
		Commands:       Commands{Up: cmdUp, Enter: cmdEnter},
		Mounts:         mounts,
		Resources:      raw.Resources,
		Envs:           envs,
		Network:        network,
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
func parseCaps(val any) (caps Caps, err error) {
	if val == nil {
		// No caps field - return empty, defaults applied in LoadConfig
		return Caps{}, nil
	}

	switch v := val.(type) {
	case []any:
		// Array mode (additive): user specifies additional caps beyond defaults
		userCaps, err := toStringSlice(v, "caps")
		if err != nil {
			return Caps{}, err
		}
		// Additive mode: drop ALL, add defaults + user caps (deduped)
		add := append([]string{}, DefaultCaps...)
		for _, cap := range userCaps {
			if !slices.Contains(DefaultCaps, cap) {
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
			caps.Drop, err = toStringSlice(drop, "caps.drop")
			if err != nil {
				return Caps{}, err
			}
		}
		if add, ok := v["add"].([]any); ok {
			caps.Add, err = toStringSlice(add, "caps.add")
			if err != nil {
				return Caps{}, err
			}
		}
		return caps, nil

	default:
		return Caps{}, fmt.Errorf("caps: expected array or object, got %T", val)
	}
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
		return EnvValue{}, fmt.Errorf("invalid type: %T: %w", val, ErrInvalidType)
	}
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

	// Clone reference types from base to avoid aliasing mutations.
	result.Envs = maps.Clone(base.Envs)
	result.Mounts = slices.Clone(base.Mounts)
	result.Network.LANAccess = slices.Clone(base.Network.LANAccess)
	result.Network.Ports = slices.Clone(base.Network.Ports)
	// Network.Proxy is a string — no cloning needed

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
	// Ports: overlay replaces if non-empty (complete specification, not append)
	if len(overlay.Network.Ports) > 0 {
		result.Network.Ports = overlay.Network.Ports
	}
	// Proxy: overlay wins if non-empty
	if overlay.Network.Proxy != "" {
		result.Network.Proxy = overlay.Network.Proxy
	}

	// Caps: overlay wins if non-empty (full replacement, not merge)
	if len(overlay.Caps.Drop) > 0 || len(overlay.Caps.Add) > 0 {
		result.Caps = overlay.Caps
	}

	return result
}

// toStringSlice converts []any to []string with error context.
func toStringSlice(items []any, context string) ([]string, error) {
	result := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d]: expected string, got %T", context, i, item)
		}
		result = append(result, s)
	}
	return result, nil
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
