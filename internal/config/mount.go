// mount.go implements mount configuration parsing and helpers.
// See AGD-025 for mount exclude implementation with Mutagen.
package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/invopop/jsonschema"
)

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
		return MountConfig{}, fmt.Errorf("invalid mount format %q: expected source:target[:ro]: %w", s, ErrInvalidMountFormat)
	}

	m := MountConfig{
		Source: parts[0],
		Target: parts[1],
	}

	if len(parts) == 3 {
		if parts[2] == "ro" {
			m.Readonly = true
		} else {
			return MountConfig{}, fmt.Errorf("invalid mount option %q: expected 'ro': %w", parts[2], ErrInvalidMountOption)
		}
	}

	if m.Source == "" {
		return MountConfig{}, fmt.Errorf("mount source cannot be empty: %w", ErrMountSourceEmpty)
	}
	if m.Target == "" {
		return MountConfig{}, fmt.Errorf("mount target cannot be empty: %w", ErrMountTargetEmpty)
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

// parseMounts converts raw mount values to MountConfig slice.
// Accepts both string format ("source:target[:ro]") and object format.
// expandEnv expands ${VAR} references in mount source paths only (not target).
func parseMounts(raw []any, expandEnv func(string) (string, error)) ([]MountConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	mounts := make([]MountConfig, 0, len(raw))
	for i, val := range raw {
		m, err := parseMountValue(val, expandEnv)
		if err != nil {
			return nil, fmt.Errorf("mount[%d]: %w", i, err)
		}
		mounts = append(mounts, m)
	}
	return mounts, nil
}

// parseMountValue converts a single raw mount value to MountConfig.
// expandEnv expands ${VAR} references in mount source paths only (not target).
func parseMountValue(val any, expandEnv func(string) (string, error)) (MountConfig, error) {
	switch v := val.(type) {
	case string:
		m, err := ParseMount(v)
		if err != nil {
			return MountConfig{}, err
		}
		source, err := expandEnv(m.Source)
		if err != nil {
			return MountConfig{}, err
		}
		m.Source = source
		return m, nil
	case map[string]any:
		return parseMountObject(v, expandEnv)
	default:
		return MountConfig{}, fmt.Errorf("invalid type: %T: %w", val, ErrInvalidType)
	}
}

// parseMountObject parses a mount object with source, target, readonly, exclude fields.
// expandEnv expands ${VAR} references in source paths only (not target).
func parseMountObject(m map[string]any, expandEnv func(string) (string, error)) (MountConfig, error) {
	var mc MountConfig

	source, ok := m["source"].(string)
	if !ok || source == "" {
		return MountConfig{}, fmt.Errorf("mount source is required")
	}
	expandedSource, err := expandEnv(source)
	if err != nil {
		return MountConfig{}, err
	}
	mc.Source = expandedSource

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
