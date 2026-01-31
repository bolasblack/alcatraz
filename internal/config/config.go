// Package config handles parsing and writing of Alcatraz configuration files (.alca.toml).
// See AGD-009 for configuration format design decisions.
// See AGD-022 for includes support and Config/RawConfig type separation.
package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/invopop/jsonschema"

	"github.com/bolasblack/alcatraz/internal/util"
)

// Commands defines the lifecycle commands for the container.
type Commands struct {
	Up    string `toml:"up,omitempty,multiline" json:"up,omitempty" jsonschema:"description=Command to run when starting the container"`
	Enter string `toml:"enter,omitempty,multiline" json:"enter,omitempty" jsonschema:"description=Command to run when entering an existing container"`
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

// DefaultWorkdir is the default working directory inside the container.
const DefaultWorkdir = "/workspace"

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

// IsInterpolated returns true if the value contains ${...} interpolation syntax.
func (e EnvValue) IsInterpolated() bool {
	return strings.Contains(e.Value, "${")
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

// MountConfig represents a mount configuration.
// See AGD-025 for mount exclude implementation with Mutagen.
type MountConfig struct {
	Source   string   `toml:"source" json:"source" jsonschema:"description=Host path (required)"`
	Target   string   `toml:"target" json:"target" jsonschema:"description=Container path (required)"`
	Readonly bool     `toml:"readonly,omitempty" json:"readonly,omitempty" jsonschema:"description=Read-only mount (default: false)"`
	Exclude  []string `toml:"exclude,omitempty" json:"exclude,omitempty" jsonschema:"description=Glob patterns to exclude (optional)"`
}

// UnmarshalJSON supports both string ("source:target[:ro]") and object formats.
// This provides backward compatibility with state files saved before MountConfig
// was changed from string to struct.
func (m *MountConfig) UnmarshalJSON(data []byte) error {
	// Try string format first (backward compat)
	var s string
	if json.Unmarshal(data, &s) == nil {
		parsed, err := ParseMount(s)
		if err != nil {
			return err
		}
		*m = parsed
		return nil
	}

	// Object format
	type mountConfigAlias MountConfig
	var alias mountConfigAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*m = MountConfig(alias)
	return nil
}

// ParseMount parses a mount string "source:target[:ro]" into MountConfig.
func ParseMount(s string) (MountConfig, error) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return MountConfig{}, fmt.Errorf("invalid mount format %q: expected source:target[:ro]", s)
	}

	m := MountConfig{
		Source: parts[0],
		Target: parts[1],
	}

	if len(parts) == 3 {
		if parts[2] == "ro" {
			m.Readonly = true
		} else {
			return MountConfig{}, fmt.Errorf("invalid mount option %q: expected 'ro'", parts[2])
		}
	}

	if m.Source == "" {
		return MountConfig{}, fmt.Errorf("mount source cannot be empty")
	}
	if m.Target == "" {
		return MountConfig{}, fmt.Errorf("mount target cannot be empty")
	}

	return m, nil
}

// String returns the mount in docker -v format.
// Returns empty string if the mount has excludes (cannot be represented in string format).
// Use CanBeSimpleString() to check before calling.
func (m MountConfig) String() string {
	// Mirror type ensures all MountConfig fields are explicitly handled (AGD-015).
	type fields struct {
		Source   string
		Target   string
		Readonly bool
		Exclude  []string
	}
	_ = fields(m)

	// Mounts with excludes cannot be represented in string format
	if m.HasExcludes() {
		return ""
	}

	result := m.Source + ":" + m.Target
	if m.Readonly {
		result += ":ro"
	}
	return result
}

// CanBeSimpleString returns true if the mount can be represented as a simple string.
// Returns false if the mount has excludes which require the extended object format.
func (m MountConfig) CanBeSimpleString() bool {
	return !m.HasExcludes()
}

// HasExcludes returns true if the mount has exclude patterns.
func (m MountConfig) HasExcludes() bool {
	return len(m.Exclude) > 0
}

// Equals compares two MountConfig for equality.
func (m MountConfig) Equals(other MountConfig) bool {
	// Mirror type ensures all MountConfig fields are explicitly handled (AGD-015).
	type fields struct {
		Source   string
		Target   string
		Readonly bool
		Exclude  []string
	}
	_ = fields(m)
	_ = fields(other)

	if m.Source != other.Source || m.Target != other.Target || m.Readonly != other.Readonly {
		return false
	}
	if len(m.Exclude) != len(other.Exclude) {
		return false
	}
	for i, e := range m.Exclude {
		if e != other.Exclude[i] {
			return false
		}
	}
	return true
}

// MountsEqual compares two slices of MountConfig for equality.
func MountsEqual(a, b []MountConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equals(b[i]) {
			return false
		}
	}
	return true
}

// Config represents the Alcatraz container configuration (after processing).
// This is the final merged config used internally by the program.
type Config struct {
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

// DefaultConfig returns a Config with sensible defaults.
// See AGD-009 for default values rationale.
func DefaultConfig() Config {
	return Config{
		Image:   "nixos/nix",
		Workdir: DefaultWorkdir,
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
	// Range over nil map is safe in Go (no-op), no nil check needed.
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

// RawMountSlice is a slice of raw mount values for RawConfig.
// Used for both TOML parsing (accepts string or object) and JSON schema generation.
type RawMountSlice []any

// JSONSchema implements jsonschema.JSONSchemer to generate correct schema.
func (RawMountSlice) JSONSchema() *jsonschema.Schema {
	mountProps := jsonschema.NewProperties()
	mountProps.Set("source", &jsonschema.Schema{Type: "string", Description: "Host path (required)"})
	mountProps.Set("target", &jsonschema.Schema{Type: "string", Description: "Container path (required)"})
	mountProps.Set("readonly", &jsonschema.Schema{Type: "boolean", Description: "Read-only mount (default: false)"})
	mountProps.Set("exclude", &jsonschema.Schema{
		Type:        "array",
		Items:       &jsonschema.Schema{Type: "string"},
		Description: "Glob patterns to exclude (optional)",
	})

	return &jsonschema.Schema{
		Type: "array",
		Items: &jsonschema.Schema{
			OneOf: []*jsonschema.Schema{
				{Type: "string", Description: "Simple format: source:target[:ro]"},
				{
					Type:                 "object",
					Properties:           mountProps,
					Required:             []string{"source", "target"},
					AdditionalProperties: jsonschema.FalseSchema,
					Description:          "Extended format with excludes",
				},
			},
		},
		Description: "Additional bind mounts",
	}
}

// RawConfig represents the raw configuration as written in .alca.toml files.
// Used for TOML parsing and JSON schema generation.
type RawConfig struct {
	Includes       []string       `toml:"includes,omitempty" json:"includes,omitempty" jsonschema:"description=Other config files to include and merge (supports glob patterns)"`
	Image          string         `toml:"image" json:"image" jsonschema:"description=Container image to use"`
	Workdir        string         `toml:"workdir,omitempty" json:"workdir,omitempty" jsonschema:"description=Working directory inside container"`
	WorkdirExclude []string       `toml:"workdir_exclude,omitempty" json:"workdir_exclude,omitempty" jsonschema:"description=Patterns to exclude from workdir mount (requires Mutagen)"`
	Runtime        RuntimeType    `toml:"runtime,omitempty" json:"runtime,omitempty" jsonschema:"enum=auto,enum=docker,description=Container runtime selection"`
	Commands       Commands       `toml:"commands,omitempty" json:"commands,omitempty" jsonschema:"description=Lifecycle commands"`
	Mounts         RawMountSlice  `toml:"mounts,omitempty" json:"mounts,omitempty"`
	Resources      Resources      `toml:"resources,omitempty" json:"resources,omitempty" jsonschema:"description=Container resource limits"`
	Envs           RawEnvValueMap `toml:"envs,omitempty" json:"envs,omitempty"`
	Network        Network        `toml:"network,omitempty" json:"network,omitempty" jsonschema:"description=Network configuration"`
}

// LoadConfig reads and parses a configuration file from the given path.
// Supports includes directive for composable configuration.
// Applies defaults for missing fields: runtime defaults to "auto", workdir to "/workspace".
// Normalizes workdir into Mounts[0] with any excludes.
func LoadConfig(env *util.Env, path string) (Config, error) {
	cfg, err := LoadWithIncludes(env, path)
	if err != nil {
		return Config{}, err
	}

	// Validate required fields
	if cfg.Image == "" {
		return Config{}, fmt.Errorf("image field is required in configuration %s", path)
	}

	// Apply defaults for missing fields
	if cfg.Runtime == "" {
		cfg.Runtime = RuntimeAuto
	}
	if cfg.Workdir == "" {
		cfg.Workdir = DefaultWorkdir
	}

	// Check for mount target conflicts with workdir
	for _, mount := range cfg.Mounts {
		if mount.Target == cfg.Workdir {
			return Config{}, fmt.Errorf("mount target %q conflicts with workdir; use workdir_exclude instead of a separate mount", cfg.Workdir)
		}
	}

	// Normalize: insert workdir as Mounts[0]
	workdirMount := MountConfig{
		Source:  ".",
		Target:  cfg.Workdir,
		Exclude: cfg.WorkdirExclude,
	}
	cfg.Mounts = append([]MountConfig{workdirMount}, cfg.Mounts...)

	return cfg, nil
}
