// generator.go provides config templates for alca init.
//
// Design principle: init and default are complementary.
// - Fields with defaults (runtime, workdir) don't need init generation
// - init primarily generates JSON schema required fields (exceptions allowed for demo/education)

package config

import (
	"bytes"
	"fmt"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Template represents a configuration template type.
type Template string

const (
	// TemplateNix generates a Nix-based configuration.
	TemplateNix Template = "nix"
	// TemplateDebian generates a Debian-based configuration.
	TemplateDebian Template = "debian"
)

// SchemaComment is the TOML comment that references the JSON Schema for editor autocomplete.
const SchemaComment = "#:schema https://raw.githubusercontent.com/bolasblack/alcatraz/refs/heads/master/alca-config.schema.json\n\n"

// TemplateConfig holds a Config and its associated comment.
type TemplateConfig struct {
	Config    Config
	Includes  []string // Config files to include (RawConfig-only field)
	UpComment string   // Comment to insert before the "up" command
}

// GenerateConfig returns the TOML content for the given template.
func GenerateConfig(template Template) (string, error) {
	tc := getTemplateConfig(template)

	raw := configToRaw(tc.Config)
	raw.Includes = tc.Includes

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return "", fmt.Errorf("encode template: %w", err)
	}

	content := buf.String()

	// Insert comment before "up" command if present
	if tc.UpComment != "" {
		content = insertUpComment(content, tc.UpComment)
	}

	return SchemaComment + content, nil
}

func getTemplateConfig(template Template) TemplateConfig {
	switch template {
	case TemplateNix:
		return TemplateConfig{
			Config: Config{
				Image: "nixos/nix",
				Mounts: []MountConfig{
					{Source: ".alca.cache/go", Target: "/root/go"},
					{Source: ".alca.cache/mise", Target: "/root/.local/share/mise"},
				},
				Commands: Commands{
					Up:    "[ -f flake.nix ] && exec nix develop --profile /nix/var/nix/profiles/devshell --command true",
					Enter: "[ -f flake.nix ] && exec nix develop --profile /nix/var/nix/profiles/devshell --command",
				},
				Envs: map[string]EnvValue{
					"NIXPKGS_ALLOW_UNFREE": {Value: "1"},
					"NIX_CONFIG":           {Value: "extra-experimental-features = nix-command flakes"},
				},
			},
			Includes:  []string{"./.alca.*.toml"},
			UpComment: "prebuild, to reduce the time costs on enter",
		}
	case TemplateDebian:
		return TemplateConfig{
			Config: Config{
				Image: "debian:bookworm-slim",
				Mounts: []MountConfig{
					{Source: ".alca.cache/go", Target: "/root/go"},
					{Source: ".alca.cache/mise", Target: "/root/.local/share/mise"},
				},
				Commands: Commands{
					Up: `apt update -y && apt install -y curl
install -dm 755 /etc/apt/keyrings
curl -fSs https://mise.jdx.dev/gpg-key.pub | tee /etc/apt/keyrings/mise-archive-keyring.asc 1> /dev/null
echo "deb [signed-by=/etc/apt/keyrings/mise-archive-keyring.asc] https://mise.jdx.dev/deb stable main" | tee /etc/apt/sources.list.d/mise.list
apt update -y
apt install -y mise

echo '
export PATH="/root/.local/share/mise/shims:$PATH"
export PATH="/extra-bin:$PATH"
' >> ~/.bashrc
. ~/.bashrc`,
				},
				Envs: map[string]EnvValue{},
			},
			Includes:  []string{"./.alca.*.toml"},
			UpComment: "prepare the environment",
		}
	default:
		return getTemplateConfig(TemplateNix)
	}
}

// insertUpComment inserts a comment before the "up" field in [commands] section.
func insertUpComment(content, comment string) string {
	// Find [commands] section and insert comment before the first field
	lines := strings.Split(content, "\n")
	var result []string
	inCommands := false

	for _, line := range lines {
		if strings.TrimSpace(line) == "[commands]" {
			inCommands = true
			result = append(result, line)
			continue
		}
		if inCommands && strings.HasPrefix(strings.TrimSpace(line), "up") {
			result = append(result, "# "+comment)
			inCommands = false
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// configToRaw converts Config to RawConfig for TOML serialization.
func configToRaw(c Config) RawConfig {
	// Mirror type ensures all Config fields are explicitly handled (AGD-015).
	// Adding a new field to Config will cause a compile error here.
	type configFields struct {
		Image     string
		Workdir   string
		Runtime   RuntimeType
		Commands  Commands
		Mounts    []MountConfig
		Resources Resources
		Envs      map[string]EnvValue
		Network   Network
	}
	_ = configFields(c)

	var envs RawEnvValueMap
	if len(c.Envs) > 0 {
		envs = make(RawEnvValueMap)
		for k, v := range c.Envs {
			envs[k] = v
		}
	}

	// Convert MountConfig to raw format
	// Use string format for simple mounts, object format for mounts with excludes
	var mounts RawMountSlice
	if len(c.Mounts) > 0 {
		mounts = make(RawMountSlice, len(c.Mounts))
		for i, m := range c.Mounts {
			if m.CanBeSimpleString() {
				// Use simple string format
				mounts[i] = m.String()
			} else {
				// Use object format for mounts with excludes
				mounts[i] = mountConfigToMap(m)
			}
		}
	}

	return RawConfig{
		Image:     c.Image,
		Workdir:   c.Workdir,
		Runtime:   c.Runtime,
		Commands:  c.Commands,
		Mounts:    mounts,
		Resources: c.Resources,
		Envs:      envs,
		Network:   c.Network,
	}
}

// mountConfigToMap converts MountConfig to map for TOML serialization.
func mountConfigToMap(m MountConfig) map[string]any {
	// Mirror type ensures all MountConfig fields are explicitly handled (AGD-015).
	type fields struct {
		Source   string
		Target   string
		Readonly bool
		Exclude  []string
	}
	_ = fields(m)

	result := map[string]any{
		"source": m.Source,
		"target": m.Target,
	}
	if m.Readonly {
		result["readonly"] = m.Readonly
	}
	if len(m.Exclude) > 0 {
		result["exclude"] = m.Exclude
	}
	return result
}
