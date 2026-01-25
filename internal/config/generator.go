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

// TemplateConfig holds a Config and its associated comment.
type TemplateConfig struct {
	Config    Config
	UpComment string // Comment to insert before the "up" command
}

// GenerateConfig returns the TOML content for the given template.
func GenerateConfig(template Template) (string, error) {
	tc := getTemplateConfig(template)

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(tc.Config); err != nil {
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
				Image:  "nixos/nix",
				Mounts: []string{".alca.cache/go:/root/go", ".alca.cache/mise:/root/.local/share/mise"},
				Commands: Commands{
					Up:    "[ -f flake.nix ] && exec nix develop --profile /nix/var/nix/profiles/devshell --command true",
					Enter: "[ -f flake.nix ] && exec nix develop --profile /nix/var/nix/profiles/devshell --command",
				},
			},
			UpComment: "prebuild, to reduce the time costs on enter",
		}
	case TemplateDebian:
		return TemplateConfig{
			Config: Config{
				Image:  "debian:bookworm-slim",
				Mounts: []string{".alca.cache/go:/root/go", ".alca.cache/mise:/root/.local/share/mise"},
				Commands: Commands{
					Up: `apt update -y && apt install -y curl
install -dm 755 /etc/apt/keyrings
curl -fSs https://mise.jdx.dev/gpg-key.pub | tee /etc/apt/keyrings/mise-archive-keyring.asc 1> /dev/null
echo "deb [signed-by=/etc/apt/keyrings/mise-archive-keyring.asc] https://mise.jdx.dev/deb stable main" | tee /etc/apt/sources.list.d/mise.list
apt update -y
apt install -y mise`,
				},
			},
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
