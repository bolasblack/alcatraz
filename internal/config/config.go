// Package config handles parsing and writing of Alcatraz configuration files (.alca.toml).
// See AGD-009 for configuration format design decisions.
package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

// Commands defines the lifecycle commands for the container.
type Commands struct {
	Up    string `toml:"up,omitempty" jsonschema:"description=Command to run when starting the container"`
	Enter string `toml:"enter,omitempty" jsonschema:"description=Command to run when entering an existing container"`
}

// Resources defines container resource limits.
type Resources struct {
	Memory string `toml:"memory,omitempty" jsonschema:"description=Memory limit (e.g. 4g or 512m)"`
	CPUs   int    `toml:"cpus,omitempty" jsonschema:"description=Number of CPUs to allocate"`
}

// RuntimeType defines the container runtime selection mode.
// See AGD-012 for runtime config design decisions.
type RuntimeType string

const (
	// RuntimeAuto auto-detects the best available runtime.
	// macOS: Apple Containerization > Docker
	// Linux: Podman > Docker
	RuntimeAuto RuntimeType = "auto"

	// RuntimeDocker forces Docker regardless of other available runtimes.
	RuntimeDocker RuntimeType = "docker"
)

// Config represents the Alcatraz container configuration.
type Config struct {
	Image     string      `toml:"image" jsonschema:"required,description=Container image to use"`
	Workdir   string      `toml:"workdir,omitempty" jsonschema:"description=Working directory inside container"`
	Runtime   RuntimeType `toml:"runtime,omitempty" jsonschema:"enum=auto,enum=docker,description=Container runtime selection"`
	Commands  Commands    `toml:"commands,omitempty" jsonschema:"description=Lifecycle commands"`
	Mounts    []string    `toml:"mounts,omitempty" jsonschema:"description=Additional bind mounts (source:target[:ro])"`
	Resources Resources   `toml:"resources,omitempty" jsonschema:"description=Container resource limits"`
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

// LoadConfig reads and parses a configuration file from the given path.
func LoadConfig(path string) (Config, error) {
	var cfg Config
	_, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// SchemaComment is the TOML comment that references the JSON Schema for editor autocomplete.
const SchemaComment = "#:schema https://raw.githubusercontent.com/bolasblack/alcatraz/refs/heads/master/alca-config.schema.json\n\n"

// SaveConfig writes the configuration to the given path with schema comment header.
func SaveConfig(path string, cfg Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write schema comment for editor autocomplete support
	if _, err := f.WriteString(SchemaComment); err != nil {
		return err
	}

	encoder := toml.NewEncoder(f)
	return encoder.Encode(cfg)
}
