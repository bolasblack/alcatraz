// generator.go provides config templates for alca init.
//
// Design principle: init and default are complementary.
// - Fields with defaults (runtime, workdir) don't need init generation
// - init primarily generates JSON schema required fields (exceptions allowed for demo/education)

package config

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/afero"
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
	Extends   []string // Config files to extend (RawConfig-only field)
	UpComment string   // Comment to insert before the "up" command
	Gitignore []string // Entries to append to .gitignore if it exists
}

// GenerateConfig writes the TOML config file and appends gitignore entries.
func GenerateConfig(fs afero.Fs, configPath string, tc TemplateConfig) error {
	content, err := generateConfigContent(tc)
	if err != nil {
		return err
	}

	if err := afero.WriteFile(fs, configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	if len(tc.Gitignore) > 0 {
		dir := filepath.Dir(configPath)
		if err := appendGitignoreEntries(fs, dir, tc.Gitignore); err != nil {
			return fmt.Errorf("update .gitignore: %w", err)
		}
	}

	return nil
}

// generateConfigContent returns the TOML content string for a TemplateConfig.
func generateConfigContent(tc TemplateConfig) (string, error) {
	raw := configToRaw(tc.Config)
	raw.Extends = tc.Extends
	raw.Includes = tc.Includes

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(raw); err != nil {
		return "", fmt.Errorf("encode template: %w", err)
	}

	content := buf.String()
	content = convertMultilineStrings(content)

	if tc.UpComment != "" {
		content = insertUpComment(content, tc.UpComment)
	}

	return SchemaComment + content, nil
}

// GetTemplateConfig returns the TemplateConfig for a given template.
func GetTemplateConfig(template Template) TemplateConfig {
	switch template {
	case TemplateNix:
		return TemplateConfig{
			Config: Config{
				Image: "nixos/nix",
				Mounts: []MountConfig{
					{Source: ".alca.mounts/mise", Target: "/root/.local/share/mise"},
				},
				Commands: Commands{
					Up:    CommandValue{Command: "[ -f flake.nix ] && exec nix develop --profile /nix/var/nix/profiles/devshell --command true"},
					Enter: CommandValue{Command: "[ -f flake.nix ] && exec nix develop --profile /nix/var/nix/profiles/devshell --command"},
				},
				Envs: map[string]EnvValue{
					"IS_SANDBOX":           {Value: "1"},
					"NIXPKGS_ALLOW_UNFREE": {Value: "1"},
					"NIX_CONFIG":           {Value: "extra-experimental-features = nix-command flakes"},
				},
				WorkdirExclude: []string{".env"},
			},
			Includes:  []string{"./.alca.*.toml"},
			UpComment: "prebuild, to reduce the time costs on enter",
			Gitignore: []string{".alca.local.toml", ".alca.mounts/"},
		}
	case TemplateDebian:
		return TemplateConfig{
			Config: Config{
				Image: "debian:bookworm-slim",
				Mounts: []MountConfig{
					{Source: ".alca.mounts/mise", Target: "/root/.local/share/mise"},
				},
				Commands: Commands{
					Up: CommandValue{Command: `apt update -y && apt install -y curl
install -dm 755 /etc/apt/keyrings
curl -fSs https://mise.jdx.dev/gpg-key.pub | tee /etc/apt/keyrings/mise-archive-keyring.asc 1> /dev/null
echo "deb [signed-by=/etc/apt/keyrings/mise-archive-keyring.asc] https://mise.jdx.dev/deb stable main" | tee /etc/apt/sources.list.d/mise.list
apt update -y
apt install -y mise

echo '
export PATH="/root/.local/share/mise/shims:$PATH"
export PATH="/extra-bin:$PATH"
' >> ~/.bashrc
. ~/.bashrc

mise trust
mise install`},
					Enter: CommandValue{Command: `. ~/.bashrc`},
				},
				Envs: map[string]EnvValue{
					"IS_SANDBOX": {Value: "1"},
				},
				WorkdirExclude: []string{".env"},
			},
			Includes:  []string{"./.alca.*.toml"},
			UpComment: "prepare the environment",
			Gitignore: []string{".alca.local.toml", ".alca.mounts/"},
		}
	default:
		// Intentional fallback: unknown templates default to Nix (tested by TestGenerateConfigUnknownTemplate)
		return GetTemplateConfig(TemplateNix)
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
	_ = configFields(c)

	var envs RawEnvValueMap
	if len(c.Envs) > 0 {
		envs = make(RawEnvValueMap)
		for k, v := range c.Envs {
			if !v.OverrideOnEnter {
				envs[k] = v.Value // simple string
			} else {
				envs[k] = v // full struct
			}
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

	// Convert Caps to raw format (object mode for explicit control)
	var caps RawCaps
	if len(c.Caps.Drop) > 0 || len(c.Caps.Add) > 0 {
		capsMap := make(map[string]any)
		if len(c.Caps.Drop) > 0 {
			drop := make([]any, len(c.Caps.Drop))
			for i, d := range c.Caps.Drop {
				drop[i] = d
			}
			capsMap["drop"] = drop
		}
		if len(c.Caps.Add) > 0 {
			add := make([]any, len(c.Caps.Add))
			for i, a := range c.Caps.Add {
				add[i] = a
			}
			capsMap["add"] = add
		}
		caps = capsMap
	}

	// Convert Commands to RawCommands (use simple string format when no append)
	var commands RawCommands
	if c.Commands.Up.Command != "" {
		commands.Up = commandValueToRaw(c.Commands.Up)
	}
	if c.Commands.Enter.Command != "" {
		commands.Enter = commandValueToRaw(c.Commands.Enter)
	}

	return RawConfig{
		Image:          c.Image,
		Workdir:        c.Workdir,
		WorkdirExclude: c.WorkdirExclude,
		Runtime:        c.Runtime,
		Commands:       commands,
		Mounts:         mounts,
		Resources:      c.Resources,
		Envs:           envs,
		Network:        c.Network,
		Caps:           caps,
	}
}

// commandValueToRaw converts CommandValue to raw format for TOML serialization.
// Uses simple string format when append is false, object format when append is true.
func commandValueToRaw(cv CommandValue) RawCommandValue {
	if cv.Append {
		return map[string]any{
			"command": cv.Command,
			"append":  true,
		}
	}
	return cv.Command
}

// multilinePattern matches TOML key-value lines where the string value contains literal \n sequences.
var multilinePattern = regexp.MustCompile(`^(\s*\S+\s*=\s*)"((?:[^"\\]|\\.)*)"\s*$`)

// convertMultilineStrings post-processes TOML output to convert string values
// containing literal \n escape sequences into TOML multiline basic strings (""").
func convertMultilineStrings(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		m := multilinePattern.FindStringSubmatch(line)
		if m != nil && strings.Contains(m[2], `\n`) {
			prefix := m[1] // e.g. "up = "
			raw := m[2]
			// Replace literal \n sequences with actual newlines,
			// and unescape \" to " (unnecessary inside triple-quoted strings)
			expanded := strings.ReplaceAll(raw, `\n`, "\n")
			expanded = strings.ReplaceAll(expanded, `\"`, `"`)
			result = append(result, prefix+"\"\"\"\n"+expanded+"\n\"\"\"")
			continue
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// appendGitignoreEntries appends entries to .gitignore if the file exists.
// It does not create .gitignore if it doesn't exist.
func appendGitignoreEntries(fs afero.Fs, dir string, entries []string) error {
	gitignorePath := filepath.Join(dir, ".gitignore")

	var content string
	if data, err := afero.ReadFile(fs, gitignorePath); err == nil {
		content = string(data)
	}

	existingLines := strings.Split(content, "\n")

	var toAdd []string
	for _, entry := range entries {
		found := false
		for _, line := range existingLines {
			if strings.TrimSpace(line) == entry {
				found = true
				break
			}
		}
		if !found {
			toAdd = append(toAdd, entry)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	// Ensure a trailing newline before appending
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	content += strings.Join(toAdd, "\n") + "\n"

	return afero.WriteFile(fs, gitignorePath, []byte(content), 0644)
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
