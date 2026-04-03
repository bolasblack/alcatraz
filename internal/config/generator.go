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
	// TemplateAlpine generates an Alpine-based configuration.
	TemplateAlpine Template = "alpine"
	// TemplateNix generates a Nix-based configuration.
	TemplateNix Template = "nix"
	// TemplateDebianMise generates a Debian+mise configuration using the pre-built alca-debian image.
	TemplateDebianMise Template = "debian-mise"
	// TemplateDebianSlim generates a plain Debian configuration with mise installed via APT on first run.
	TemplateDebianSlim Template = "debian-slim"
)

// LLMsComment is the TOML comment that points LLMs to the project's llms.txt.
const LLMsComment = "# llms.txt: https://bolasblack.github.io/alcatraz/llms.txt\n"

// SchemaComment is the TOML comment that references the JSON Schema for editor autocomplete.
const SchemaComment = "#:schema https://raw.githubusercontent.com/bolasblack/alcatraz/refs/heads/master/alca-config.schema.json\n\n"

// TemplateConfig holds a Config and its associated comment.
type TemplateConfig struct {
	Config       Config
	Includes     []string // Config files to include (RawConfig-only field)
	Extends      []string // Config files to extend (RawConfig-only field)
	ImageComment string   // Comment to insert before the "image" field
	UpComment    string   // Comment to insert before the "up" command
	Gitignore    []string // Entries to append to .gitignore if it exists
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

	if tc.ImageComment != "" {
		content = insertImageComment(content, tc.ImageComment)
	}
	if tc.UpComment != "" {
		content = insertUpComment(content, tc.UpComment)
	}

	return LLMsComment + SchemaComment + content, nil
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
				WorkdirExclude: []string{".env", ".alca.mounts"},
			},
			Includes:  []string{"./.alca.*.toml"},
			UpComment: "prebuild, to reduce the time costs on enter",
			Gitignore: []string{".alca.local.toml", ".alca.*.local.toml", ".alca/", ".alca.mounts/"},
		}
	case TemplateAlpine:
		return TemplateConfig{
			Config: Config{
				Image: "alpine:3.21",
				Mounts: []MountConfig{
					{Source: ".alca.mounts/mise", Target: "/root/.local/share/mise"},
					{Source: ".alca.mounts/extra-bin", Target: "/extra-bin"},
					{Source: ".alca.mounts/extra-scripts", Target: "/extra-scripts"},
				},
				Commands: Commands{
					Up: CommandValue{Command: `apk add --no-cache mise

echo '
export PATH="/root/.local/share/mise/shims:$PATH"
export PATH="/extra-bin:$PATH"
' >> ~/.profile
. ~/.profile

[ -x /extra-scripts/source.sh ] && . /extra-scripts/source.sh

mise trust -a
mise install

[ -x /extra-scripts/init.sh ] && /extra-scripts/init.sh`},
					Enter: CommandValue{Command: "[ -x /extra-scripts/source.sh ] && . /extra-scripts/source.sh\n. ~/.profile"},
				},
				Envs: map[string]EnvValue{
					"IS_SANDBOX": {Value: "1"},
				},
				WorkdirExclude: []string{".env", ".alca.mounts"},
			},
			Includes:  []string{"./.alca.*.toml"},
			UpComment: "prepare the environment",
			Gitignore: []string{".alca/", ".alca.local.toml", ".alca.*.local.toml", ".alca.mounts/"},
		}
	case TemplateDebianMise:
		return TemplateConfig{
			Config: Config{
				Image: "ghcr.io/bolasblack/alca-debian:latest",
				Mounts: []MountConfig{
					{Source: ".alca.mounts/mise", Target: "/mise"},
					{Source: ".alca.mounts/extra-bin", Target: "/extra-bin"},
					{Source: ".alca.mounts/extra-scripts", Target: "/extra-scripts"},
				},
				Commands: Commands{
					Up: CommandValue{Command: `echo '
export PATH="/extra-bin:$PATH"
' >> ~/.bashrc
. ~/.bashrc

[ -x /extra-scripts/source.sh ] && source /extra-scripts/source.sh

mise trust -a
mise install

[ -x /extra-scripts/init.sh ] && /extra-scripts/init.sh`},
					Enter: CommandValue{Command: "[ -x /extra-scripts/source.sh ] && source /extra-scripts/source.sh\n. ~/.bashrc\n"},
				},
				Envs: map[string]EnvValue{
					"IS_SANDBOX": {Value: "1"},
				},
				WorkdirExclude: []string{".env", ".alca.mounts"},
			},
			ImageComment: "Dockerfile: https://github.com/bolasblack/alcatraz/blob/master/dockerfiles/debian-mise.dockerfile\nCI/CD: https://github.com/bolasblack/alcatraz/blob/master/.github/workflows/docker-build.yml",
			Includes:     []string{"./.alca.*.toml"},
			UpComment:    "prepare the environment",
			Gitignore:    []string{".alca/", ".alca.local.toml", ".alca.*.local.toml", ".alca.mounts/"},
		}
	case TemplateDebianSlim:
		return TemplateConfig{
			Config: Config{
				Image: "debian:bookworm-slim",
				Mounts: []MountConfig{
					{Source: ".alca.mounts/extra-bin", Target: "/extra-bin"},
					{Source: ".alca.mounts/extra-scripts", Target: "/extra-scripts"},
				},
				Commands: Commands{
					Up: CommandValue{Command: `echo '
export PATH="/extra-bin:$PATH"
' >> ~/.bashrc
. ~/.bashrc

[ -x /extra-scripts/source.sh ] && source /extra-scripts/source.sh

[ -x /extra-scripts/init.sh ] && /extra-scripts/init.sh`},
					Enter: CommandValue{Command: "[ -x /extra-scripts/source.sh ] && source /extra-scripts/source.sh\n. ~/.bashrc\n"},
				},
				Envs: map[string]EnvValue{
					"IS_SANDBOX": {Value: "1"},
				},
				WorkdirExclude: []string{".env", ".alca.mounts"},
			},
			Includes:  []string{"./.alca.*.toml"},
			UpComment: "prepare the environment",
			Gitignore: []string{".alca/", ".alca.local.toml", ".alca.*.local.toml", ".alca.mounts/"},
		}
	default:
		// Intentional fallback: unknown templates default to Alpine (tested by TestGetTemplateConfigUnknownFallback)
		return GetTemplateConfig(TemplateAlpine)
	}
}

// insertImageComment inserts a comment before the "image" field.
func insertImageComment(content, comment string) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "image") {
			for _, cl := range strings.Split(comment, "\n") {
				result = append(result, "# "+cl)
			}
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n")
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
		Mounts:         mountsToRaw(c.Mounts),
		Resources:      c.Resources,
		Envs:           envsToRaw(c.Envs),
		Network:        networkToRaw(c.Network),
		Caps:           capsToRaw(c.Caps),
	}
}

// envsToRaw converts EnvValue map to raw format for TOML serialization.
// Simple values use string format; values with OverrideOnEnter use full struct.
func envsToRaw(envs map[string]EnvValue) RawEnvValueMap {
	if len(envs) == 0 {
		return nil
	}
	raw := make(RawEnvValueMap, len(envs))
	for k, v := range envs {
		if !v.OverrideOnEnter {
			raw[k] = v.Value
		} else {
			raw[k] = v
		}
	}
	return raw
}

// mountsToRaw converts MountConfig slice to raw format for TOML serialization.
// Simple mounts use string format; mounts with excludes use object format.
func mountsToRaw(mounts []MountConfig) RawMountSlice {
	if len(mounts) == 0 {
		return nil
	}
	raw := make(RawMountSlice, len(mounts))
	for i, m := range mounts {
		if m.CanBeSimpleString() {
			raw[i] = m.String()
		} else {
			raw[i] = mountConfigToMap(m)
		}
	}
	return raw
}

// networkToRaw converts Network to RawNetwork for TOML serialization.
// Ports are kept in object form (map[string]any) for TOML round-trip fidelity.
func networkToRaw(n Network) RawNetwork {
	// Mirror type ensures all Network fields are explicitly handled (AGD-015).
	type networkFields struct {
		LANAccess []string
		Ports     []PortConfig
		Proxy     string
	}
	_ = networkFields(n)

	var rawPorts RawPortSlice
	if len(n.Ports) > 0 {
		rawPorts = make(RawPortSlice, len(n.Ports))
		for i, p := range n.Ports {
			rawPorts[i] = portConfigToMap(p)
		}
	}
	return RawNetwork{
		LANAccess: n.LANAccess,
		Ports:     rawPorts,
		Proxy:     n.Proxy,
	}
}

// portConfigToMap converts a PortConfig to map[string]any for TOML serialization.
func portConfigToMap(p PortConfig) map[string]any {
	m := map[string]any{
		"port": int64(p.Port),
	}
	if p.HostIP != "" {
		m["hostIp"] = p.HostIP
	}
	if p.HostPort != 0 {
		m["hostPort"] = int64(p.HostPort)
	}
	if p.Protocol != "" {
		m["protocol"] = p.Protocol
	}
	return m
}

// capsToRaw converts Caps to raw format (object mode) for TOML serialization.
func capsToRaw(caps Caps) RawCaps {
	if len(caps.Drop) == 0 && len(caps.Add) == 0 {
		return nil
	}
	capsMap := make(map[string]any)
	if len(caps.Drop) > 0 {
		drop := make([]any, len(caps.Drop))
		for i, d := range caps.Drop {
			drop[i] = d
		}
		capsMap["drop"] = drop
	}
	if len(caps.Add) > 0 {
		add := make([]any, len(caps.Add))
		for i, a := range caps.Add {
			add[i] = a
		}
		capsMap["add"] = add
	}
	return capsMap
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
