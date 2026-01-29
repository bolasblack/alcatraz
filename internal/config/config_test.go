package config

import (
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// newTestEnv creates a test environment with in-memory filesystem.
func newTestEnv(t *testing.T) (*util.Env, afero.Fs) {
	t.Helper()
	memFs := afero.NewMemMapFs()
	env := &util.Env{Fs: memFs}
	return env, memFs
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Image != "nixos/nix" {
		t.Errorf("expected image 'nixos/nix', got %q", cfg.Image)
	}
	if cfg.Workdir != "/workspace" {
		t.Errorf("expected workdir '/workspace', got %q", cfg.Workdir)
	}
	expectedEnter := "[ -f flake.nix ] && exec nix develop"
	if cfg.Commands.Enter != expectedEnter {
		t.Errorf("expected commands.enter %q, got %q", expectedEnter, cfg.Commands.Enter)
	}
}

func TestLoadConfig(t *testing.T) {
	content := `
image = "ubuntu:latest"
workdir = "/app"
mounts = ["/host:/container", "/data:/data"]

[commands]
up = "apt update"
enter = "bash"
`
	env, memFs := newTestEnv(t)
	path := "/test/.alca.toml"
	if err := afero.WriteFile(memFs, path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(env, path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Image != "ubuntu:latest" {
		t.Errorf("expected image 'ubuntu:latest', got %q", cfg.Image)
	}
	if cfg.Workdir != "/app" {
		t.Errorf("expected workdir '/app', got %q", cfg.Workdir)
	}
	if cfg.Commands.Up != "apt update" {
		t.Errorf("expected commands.up 'apt update', got %q", cfg.Commands.Up)
	}
	if cfg.Commands.Enter != "bash" {
		t.Errorf("expected commands.enter 'bash', got %q", cfg.Commands.Enter)
	}
	if len(cfg.Mounts) != 2 {
		t.Errorf("expected 2 mounts, got %d", len(cfg.Mounts))
	}
}

func TestLoadConfigNotFound(t *testing.T) {
	env, _ := newTestEnv(t)
	_, err := LoadConfig(env, "/nonexistent/path/.alca.toml")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestLoadConfigWithEnvs(t *testing.T) {
	content := `
image = "ubuntu:latest"

[envs]
SIMPLE = "value1"
REFERENCE = "${HOST_VAR}"

[envs.COMPLEX]
value = "value2"
override_on_enter = true
`
	env, memFs := newTestEnv(t)
	path := "/test/.alca.toml"
	if err := afero.WriteFile(memFs, path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(env, path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Test simple string env
	if e, ok := cfg.Envs["SIMPLE"]; !ok {
		t.Error("expected SIMPLE env to exist")
	} else if e.Value != "value1" || e.OverrideOnEnter {
		t.Errorf("SIMPLE env: got value=%q override=%v, want value='value1' override=false",
			e.Value, e.OverrideOnEnter)
	}

	// Test reference env
	if e, ok := cfg.Envs["REFERENCE"]; !ok {
		t.Error("expected REFERENCE env to exist")
	} else if e.Value != "${HOST_VAR}" {
		t.Errorf("REFERENCE env: got value=%q, want '${HOST_VAR}'", e.Value)
	}

	// Test complex object env
	if e, ok := cfg.Envs["COMPLEX"]; !ok {
		t.Error("expected COMPLEX env to exist")
	} else if e.Value != "value2" || !e.OverrideOnEnter {
		t.Errorf("COMPLEX env: got value=%q override=%v, want value='value2' override=true",
			e.Value, e.OverrideOnEnter)
	}
}

func TestEnvValueValidate(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"static value", "hello", false},
		{"valid reference", "${MY_VAR}", false},
		{"valid with underscore", "${MY_VAR_NAME}", false},
		{"valid with hyphen", "${MY-VAR}", false},
		{"valid with numbers", "${VAR123}", false},
		{"invalid nested braces", "${${VAR}}", true},
		{"invalid partial syntax", "prefix${VAR}suffix", true},
		{"invalid missing closing brace", "${VAR", true},
		{"invalid empty var name", "${}", true},
		{"invalid starts with number", "${123VAR}", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := EnvValue{Value: tt.value}
			err := env.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnvValueExpand(t *testing.T) {
	mockEnv := func(key string) string {
		envs := map[string]string{
			"HOME":   "/home/user",
			"USER":   "testuser",
			"EMPTY":  "",
			"MY_VAR": "myvalue",
		}
		return envs[key]
	}

	tests := []struct {
		name   string
		value  string
		expect string
	}{
		{"static value", "hello", "hello"},
		{"expand HOME", "${HOME}", "/home/user"},
		{"expand USER", "${USER}", "testuser"},
		{"expand empty var", "${EMPTY}", ""},
		{"expand undefined var", "${UNDEFINED}", ""},
		{"expand MY_VAR", "${MY_VAR}", "myvalue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := EnvValue{Value: tt.value}
			got := env.Expand(mockEnv)
			if got != tt.expect {
				t.Errorf("Expand() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestEnvValueIsInterpolated(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		expect bool
	}{
		{"static value", "hello", false},
		{"with interpolation", "${VAR}", true},
		{"partial interpolation", "prefix${VAR}", true},
		{"no dollar", "VAR", false},
		{"dollar without brace", "$VAR", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := EnvValue{Value: tt.value}
			got := env.IsInterpolated()
			if got != tt.expect {
				t.Errorf("IsInterpolated() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestConfigNormalizeRuntime(t *testing.T) {
	tests := []struct {
		name    string
		runtime RuntimeType
		expect  RuntimeType
	}{
		{"empty defaults to auto", "", RuntimeAuto},
		{"auto stays auto", RuntimeAuto, RuntimeAuto},
		{"docker stays docker", RuntimeDocker, RuntimeDocker},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{Runtime: tt.runtime}
			got := cfg.NormalizeRuntime()
			if got != tt.expect {
				t.Errorf("NormalizeRuntime() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestConfigMergedEnvs(t *testing.T) {
	t.Run("nil envs returns defaults", func(t *testing.T) {
		cfg := Config{Envs: nil}
		merged := cfg.MergedEnvs()

		// Check some default envs exist
		if _, ok := merged["TERM"]; !ok {
			t.Error("expected TERM in merged envs")
		}
		if _, ok := merged["LANG"]; !ok {
			t.Error("expected LANG in merged envs")
		}
	})

	t.Run("user envs override defaults", func(t *testing.T) {
		cfg := Config{
			Envs: map[string]EnvValue{
				"TERM":   {Value: "custom-term", OverrideOnEnter: false},
				"CUSTOM": {Value: "custom-value"},
			},
		}
		merged := cfg.MergedEnvs()

		// User TERM overrides default
		if merged["TERM"].Value != "custom-term" {
			t.Errorf("expected TERM='custom-term', got %q", merged["TERM"].Value)
		}
		if merged["TERM"].OverrideOnEnter != false {
			t.Error("expected TERM.OverrideOnEnter=false")
		}

		// Custom env is added
		if merged["CUSTOM"].Value != "custom-value" {
			t.Errorf("expected CUSTOM='custom-value', got %q", merged["CUSTOM"].Value)
		}

		// Default LANG is preserved
		if _, ok := merged["LANG"]; !ok {
			t.Error("expected LANG in merged envs")
		}
	})
}

func TestConfigValidateEnvs(t *testing.T) {
	t.Run("valid envs", func(t *testing.T) {
		cfg := Config{
			Envs: map[string]EnvValue{
				"STATIC":    {Value: "value"},
				"REFERENCE": {Value: "${HOME}"},
			},
		}
		if err := cfg.ValidateEnvs(); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("invalid env", func(t *testing.T) {
		cfg := Config{
			Envs: map[string]EnvValue{
				"BAD": {Value: "prefix${VAR}suffix"},
			},
		}
		err := cfg.ValidateEnvs()
		if err == nil {
			t.Error("expected error for invalid env")
		}
		if !strings.Contains(err.Error(), "BAD") {
			t.Errorf("expected error to mention 'BAD', got %v", err)
		}
	})
}

func TestGenerateConfig(t *testing.T) {
	tests := []struct {
		name     string
		template Template
		wantImg  string
		wantCmt  string
	}{
		{"nix", TemplateNix, "nixos/nix", "# prebuild, to reduce the time costs on enter"},
		{"debian", TemplateDebian, "debian:bookworm-slim", "# prepare the environment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := GenerateConfig(tt.template)
			if err != nil {
				t.Fatalf("GenerateConfig failed: %v", err)
			}

			if !strings.Contains(content, tt.wantImg) {
				t.Errorf("expected image %q in output", tt.wantImg)
			}
			if !strings.Contains(content, tt.wantCmt) {
				t.Errorf("expected comment %q in output", tt.wantCmt)
			}
			if !strings.Contains(content, SchemaComment) {
				t.Error("expected schema comment in output")
			}
		})
	}
}

func TestGenerateConfigUnknownTemplate(t *testing.T) {
	// Unknown template should fall back to nix
	content, err := GenerateConfig("unknown")
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}
	if !strings.Contains(content, "nixos/nix") {
		t.Error("expected unknown template to fall back to nix")
	}
}

func TestInsertUpComment(t *testing.T) {
	tests := []struct {
		name    string
		content string
		comment string
		want    string
	}{
		{
			name: "inserts comment before up",
			content: `image = "test"

[commands]
up = "test up"
enter = "test enter"
`,
			comment: "my comment",
			want: `image = "test"

[commands]
# my comment
up = "test up"
enter = "test enter"
`,
		},
		{
			name: "no commands section",
			content: `image = "test"
`,
			comment: "my comment",
			want: `image = "test"
`,
		},
		{
			name: "commands without up",
			content: `image = "test"

[commands]
enter = "bash"
`,
			comment: "my comment",
			want: `image = "test"

[commands]
enter = "bash"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := insertUpComment(tt.content, tt.comment)
			if got != tt.want {
				t.Errorf("insertUpComment() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
