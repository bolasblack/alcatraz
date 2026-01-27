// Package config handles parsing and writing of Alcatraz configuration files (.alca.toml).
// See AGD-009 for configuration format design decisions.
// See AGD-022 for includes support and Config/RawConfig type separation.
package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/invopop/jsonschema"
)

// Commands defines the lifecycle commands for the container.
type Commands struct {
	Up    string `toml:"up,omitempty" json:"up,omitempty" jsonschema:"description=Command to run when starting the container"`
	Enter string `toml:"enter,omitempty" json:"enter,omitempty" jsonschema:"description=Command to run when entering an existing container"`
}

// Resources defines container resource limits.
type Resources struct {
	Memory string `toml:"memory,omitempty" json:"memory,omitempty" jsonschema:"description=Memory limit (e.g. 4g or 512m)"`
	CPUs   int    `toml:"cpus,omitempty" json:"cpus,omitempty" jsonschema:"description=Number of CPUs to allocate"`
}

// RuntimeType defines the container runtime selection mode.
// See AGD-012 for runtime config design decisions.
type RuntimeType string

const (
	// RuntimeAuto auto-detects the best available runtime.
	// Linux: Podman > Docker
	// macOS and others: Docker
	RuntimeAuto RuntimeType = "auto"

	// RuntimeDocker forces Docker regardless of other available runtimes.
	RuntimeDocker RuntimeType = "docker"
)

// EnvValue represents an environment variable configuration.
// Can be unmarshaled from either a string or an object with value and override_on_enter fields.
// See AGD-017 for environment variable configuration design.
type EnvValue struct {
	Value           string `toml:"value" json:"value" jsonschema:"description=The value or ${VAR} reference"`
	OverrideOnEnter bool   `toml:"override_on_enter,omitempty" json:"override_on_enter,omitempty" jsonschema:"description=Also set at docker exec time"`
}


// envVarPattern matches simple ${VAR} syntax.
var envVarPattern = regexp.MustCompile(`^\$\{([a-zA-Z_][a-zA-Z0-9_-]*)\}$`)

// Validate checks if the value uses valid ${VAR} syntax.
func (e *EnvValue) Validate() error {
	if !strings.Contains(e.Value, "${") {
		return nil // Static value, always valid
	}
	if !envVarPattern.MatchString(e.Value) {
		return fmt.Errorf("invalid env value %q: only simple ${VAR} syntax supported", e.Value)
	}
	return nil
}

// Expand expands ${VAR} from the given environment lookup function.
func (e *EnvValue) Expand(getenv func(string) string) string {
	matches := envVarPattern.FindStringSubmatch(e.Value)
	if matches == nil {
		return e.Value // Static value, return as-is
	}
	return getenv(matches[1])
}

// RawEnvValue is used in RawConfig for TOML parsing.
// Underlying type is any to support flexible TOML decoding (string or object).
// Implements JSONSchema to generate correct schema for editor autocomplete.
type RawEnvValue = any

// RawEnvValueMap is a map of environment variables for RawConfig.
// Used for both TOML parsing (accepts any) and JSON schema generation.
type RawEnvValueMap map[string]RawEnvValue

// JSONSchema implements jsonschema.JSONSchemer to generate correct schema.
func (RawEnvValueMap) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:                 "object",
		AdditionalProperties: envValueSchema(),
		Description:          "Environment variables for the container",
	}
}

// envValueSchema returns the JSON schema for an environment variable value.
func envValueSchema() *jsonschema.Schema {
	props := jsonschema.NewProperties()
	props.Set("value", &jsonschema.Schema{Type: "string", Description: "The value or ${VAR} reference"})
	props.Set("override_on_enter", &jsonschema.Schema{Type: "boolean", Description: "Also set at docker exec time"})

	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{Type: "string", Description: "Static value or ${VAR} reference"},
			{
				Type:                 "object",
				Properties:           props,
				AdditionalProperties: jsonschema.FalseSchema,
			},
		},
		Description: "Environment variable value (string or object with override_on_enter)",
	}
}

// DefaultEnvs returns the built-in default environment variables.
// All defaults read from host environment with override_on_enter=true.
// See AGD-017 for rationale.
func DefaultEnvs() map[string]EnvValue {
	return map[string]EnvValue{
		"TERM":        {Value: "${TERM}", OverrideOnEnter: true},
		"COLORTERM":   {Value: "${COLORTERM}", OverrideOnEnter: true},
		"LANG":        {Value: "${LANG}", OverrideOnEnter: true},
		"LC_ALL":      {Value: "${LC_ALL}", OverrideOnEnter: true},
		"LC_COLLATE":  {Value: "${LC_COLLATE}", OverrideOnEnter: true},
		"LC_CTYPE":    {Value: "${LC_CTYPE}", OverrideOnEnter: true},
		"LC_MESSAGES": {Value: "${LC_MESSAGES}", OverrideOnEnter: true},
		"LC_MONETARY": {Value: "${LC_MONETARY}", OverrideOnEnter: true},
		"LC_NUMERIC":  {Value: "${LC_NUMERIC}", OverrideOnEnter: true},
		"LC_TIME":     {Value: "${LC_TIME}", OverrideOnEnter: true},
	}
}

// Network defines network configuration for the container.
// See AGD-023 for LAN access design decisions.
type Network struct {
	LANAccess []string `toml:"lan-access,omitempty" json:"lan-access,omitempty" jsonschema:"description=LAN access configuration (currently only '*' is supported)"`
}

// Config represents the Alcatraz container configuration (after processing).
// This is the final merged config used internally by the program.
type Config struct {
	Image     string
	Workdir   string
	Runtime   RuntimeType
	Commands  Commands
	Mounts    []string
	Resources Resources
	Envs      map[string]EnvValue
	Network   Network
}

// DefaultConfig returns a Config with sensible defaults.
// See AGD-009 for default values rationale.
func DefaultConfig() Config {
	return Config{
		Image:   "nixos/nix",
		Workdir: "/workspace",
		Runtime: RuntimeAuto,
		Commands: Commands{
			// Auto-enter nix develop if flake.nix exists
			Enter: "[ -f flake.nix ] && exec nix develop",
		},
	}
}


// NormalizeRuntime returns the runtime type, defaulting to auto if empty.
func (c *Config) NormalizeRuntime() RuntimeType {
	if c.Runtime == "" {
		return RuntimeAuto
	}
	return c.Runtime
}

// MergedEnvs returns the environment variables with defaults merged.
// User-defined values override defaults.
func (c *Config) MergedEnvs() map[string]EnvValue {
	merged := DefaultEnvs()
	for key, val := range c.Envs {
		merged[key] = val
	}
	return merged
}

// ValidateEnvs validates all environment variable configurations.
func (c *Config) ValidateEnvs() error {
	for key, env := range c.MergedEnvs() {
		if err := env.Validate(); err != nil {
			return fmt.Errorf("env %s: %w", key, err)
		}
	}
	return nil
}

// RawConfig represents the raw configuration as written in .alca.toml files.
// Used for TOML parsing and JSON schema generation.
type RawConfig struct {
	Includes  []string       `toml:"includes,omitempty" json:"includes,omitempty" jsonschema:"description=Other config files to include and merge (supports glob patterns)"`
	Image     string         `toml:"image" json:"image" jsonschema:"description=Container image to use"`
	Workdir   string         `toml:"workdir,omitempty" json:"workdir,omitempty" jsonschema:"description=Working directory inside container"`
	Runtime   RuntimeType    `toml:"runtime,omitempty" json:"runtime,omitempty" jsonschema:"enum=auto,enum=docker,description=Container runtime selection"`
	Commands  Commands       `toml:"commands,omitempty" json:"commands,omitempty" jsonschema:"description=Lifecycle commands"`
	Mounts    []string       `toml:"mounts,omitempty" json:"mounts,omitempty" jsonschema:"description=Additional bind mounts (source:target[:ro])"`
	Resources Resources      `toml:"resources,omitempty" json:"resources,omitempty" jsonschema:"description=Container resource limits"`
	Envs      RawEnvValueMap `toml:"envs,omitempty" json:"envs,omitempty"`
	Network   Network        `toml:"network,omitempty" json:"network,omitempty" jsonschema:"description=Network configuration"`
}

// LoadConfig reads and parses a configuration file from the given path.
// Supports includes directive for composable configuration.
// Applies defaults for missing fields: runtime defaults to "auto", workdir to "/workspace".
func LoadConfig(path string) (Config, error) {
	cfg, err := LoadWithIncludes(path)
	if err != nil {
		return Config{}, err
	}

	// Validate required fields
	if cfg.Image == "" {
		return Config{}, fmt.Errorf("image field is required in the final merged .alca.toml configuration")
	}

	// Apply defaults for missing fields
	if cfg.Runtime == "" {
		cfg.Runtime = RuntimeAuto
	}
	if cfg.Workdir == "" {
		cfg.Workdir = "/workspace"
	}

	return cfg, nil
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
