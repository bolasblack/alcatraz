// Package config handles parsing and writing of Alcatraz configuration files (.alca.toml).
// See AGD-009 for configuration format design decisions.
package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

// Commands defines the lifecycle commands for the container.
type Commands struct {
	Up    string `toml:"up,omitempty"`
	Enter string `toml:"enter,omitempty"`
}

// Resources defines container resource limits.
type Resources struct {
	Memory string `toml:"memory,omitempty"` // e.g. "4g", "512m"
	CPUs   int    `toml:"cpus,omitempty"`   // e.g. 2, 4
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

	// RuntimeAppleContainerization forces Apple Containerization (macOS 26+).
	RuntimeAppleContainerization RuntimeType = "apple-containerization"
)

// Config represents the Alcatraz container configuration.
type Config struct {
	Image     string      `toml:"image"`
	Workdir   string      `toml:"workdir,omitempty"`
	Runtime   RuntimeType `toml:"runtime,omitempty"`
	Commands  Commands    `toml:"commands,omitempty"`
	Mounts    []string    `toml:"mounts,omitempty"`
	Resources Resources   `toml:"resources,omitempty"`
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

// SaveConfig writes the configuration to the given path.
func SaveConfig(path string, cfg Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(cfg)
}
