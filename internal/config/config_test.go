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
	if cfg.Commands.Up.Command != "apt update" {
		t.Errorf("expected commands.up 'apt update', got %q", cfg.Commands.Up.Command)
	}
	if cfg.Commands.Enter.Command != "bash" {
		t.Errorf("expected commands.enter 'bash', got %q", cfg.Commands.Enter.Command)
	}
	// Mounts[0] is the workdir mount (normalized), user mounts follow
	if len(cfg.Mounts) != 3 {
		t.Errorf("expected 3 mounts (workdir + 2 user), got %d", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Source != "." || cfg.Mounts[0].Target != "/app" {
		t.Errorf("expected mount[0] to be workdir .:/app, got %v", cfg.Mounts[0])
	}
	if cfg.Mounts[1].Source != "/host" || cfg.Mounts[1].Target != "/container" {
		t.Errorf("expected mount[1] to be /host:/container, got %v", cfg.Mounts[1])
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

func TestRawEnvValueMapJSONSchema(t *testing.T) {
	schema := RawEnvValueMap{}.JSONSchema()

	// Verify top-level schema structure
	if schema.Type != "object" {
		t.Errorf("expected type 'object', got %q", schema.Type)
	}

	if schema.Description != "Environment variables for the container" {
		t.Errorf("expected description 'Environment variables for the container', got %q", schema.Description)
	}

	// Verify AdditionalProperties contains the env value schema
	if schema.AdditionalProperties == nil {
		t.Fatal("expected AdditionalProperties to be set")
	}

	envSchema := schema.AdditionalProperties
	if envSchema.OneOf == nil || len(envSchema.OneOf) != 2 {
		t.Fatalf("expected OneOf with 2 schemas, got %v", envSchema.OneOf)
	}

	// First option should be string
	strSchema := envSchema.OneOf[0]
	if strSchema.Type != "string" {
		t.Errorf("expected first OneOf to be string, got %q", strSchema.Type)
	}

	// Second option should be object with value and override_on_enter properties
	objSchema := envSchema.OneOf[1]
	if objSchema.Type != "object" {
		t.Errorf("expected second OneOf to be object, got %q", objSchema.Type)
	}

	if objSchema.Properties == nil {
		t.Fatal("expected Properties to be set on object schema")
	}

	// Check value property exists
	valueProp := objSchema.Properties.GetPair("value")
	if valueProp == nil {
		t.Error("expected 'value' property in object schema")
	}

	// Check override_on_enter property exists
	overrideProp := objSchema.Properties.GetPair("override_on_enter")
	if overrideProp == nil {
		t.Error("expected 'override_on_enter' property in object schema")
	}

	// Verify AdditionalProperties is false (strict schema)
	if objSchema.AdditionalProperties == nil {
		t.Error("expected AdditionalProperties to be set to false schema")
	}
}

func TestParseMount(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     MountConfig
		wantErr  bool
		errMatch string
	}{
		{
			name:  "simple mount",
			input: "/host:/container",
			want:  MountConfig{Source: "/host", Target: "/container"},
		},
		{
			name:  "readonly mount",
			input: "/host:/container:ro",
			want:  MountConfig{Source: "/host", Target: "/container", Readonly: true},
		},
		{
			name:  "relative paths",
			input: "./cache:/root/.cache",
			want:  MountConfig{Source: "./cache", Target: "/root/.cache"},
		},
		{
			name:     "too few parts",
			input:    "/host",
			wantErr:  true,
			errMatch: "invalid mount format",
		},
		{
			name:     "too many parts",
			input:    "/a:/b:/c:/d",
			wantErr:  true,
			errMatch: "invalid mount format",
		},
		{
			name:     "invalid option",
			input:    "/host:/container:rw",
			wantErr:  true,
			errMatch: "invalid mount option",
		},
		{
			name:     "empty source",
			input:    ":/container",
			wantErr:  true,
			errMatch: "source cannot be empty",
		},
		{
			name:     "empty target",
			input:    "/host:",
			wantErr:  true,
			errMatch: "target cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMount(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMount() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("ParseMount() error = %v, want error containing %q", err, tt.errMatch)
				}
				return
			}
			if !got.Equals(tt.want) {
				t.Errorf("ParseMount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMountConfigString(t *testing.T) {
	tests := []struct {
		name  string
		mount MountConfig
		want  string
	}{
		{
			name:  "simple mount",
			mount: MountConfig{Source: "/host", Target: "/container"},
			want:  "/host:/container",
		},
		{
			name:  "readonly mount",
			mount: MountConfig{Source: "/host", Target: "/container", Readonly: true},
			want:  "/host:/container:ro",
		},
		{
			name:  "mount with excludes returns empty string",
			mount: MountConfig{Source: "/host", Target: "/container", Exclude: []string{"*.tmp"}},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.mount.String()
			if got != tt.want {
				t.Errorf("MountConfig.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMountConfigEquals(t *testing.T) {
	tests := []struct {
		name string
		a, b MountConfig
		want bool
	}{
		{
			name: "equal simple mounts",
			a:    MountConfig{Source: "/a", Target: "/b"},
			b:    MountConfig{Source: "/a", Target: "/b"},
			want: true,
		},
		{
			name: "different source",
			a:    MountConfig{Source: "/a", Target: "/b"},
			b:    MountConfig{Source: "/c", Target: "/b"},
			want: false,
		},
		{
			name: "different target",
			a:    MountConfig{Source: "/a", Target: "/b"},
			b:    MountConfig{Source: "/a", Target: "/c"},
			want: false,
		},
		{
			name: "different readonly",
			a:    MountConfig{Source: "/a", Target: "/b", Readonly: true},
			b:    MountConfig{Source: "/a", Target: "/b", Readonly: false},
			want: false,
		},
		{
			name: "equal with excludes",
			a:    MountConfig{Source: "/a", Target: "/b", Exclude: []string{"*.tmp", "*.log"}},
			b:    MountConfig{Source: "/a", Target: "/b", Exclude: []string{"*.tmp", "*.log"}},
			want: true,
		},
		{
			name: "different excludes length",
			a:    MountConfig{Source: "/a", Target: "/b", Exclude: []string{"*.tmp"}},
			b:    MountConfig{Source: "/a", Target: "/b", Exclude: []string{"*.tmp", "*.log"}},
			want: false,
		},
		{
			name: "different excludes content",
			a:    MountConfig{Source: "/a", Target: "/b", Exclude: []string{"*.tmp"}},
			b:    MountConfig{Source: "/a", Target: "/b", Exclude: []string{"*.log"}},
			want: false,
		},
		{
			name: "nil vs empty excludes",
			a:    MountConfig{Source: "/a", Target: "/b", Exclude: nil},
			b:    MountConfig{Source: "/a", Target: "/b", Exclude: []string{}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.a.Equals(tt.b)
			if got != tt.want {
				t.Errorf("MountConfig.Equals() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMountsEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []MountConfig
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "nil vs empty",
			a:    nil,
			b:    []MountConfig{},
			want: true,
		},
		{
			name: "equal slices",
			a:    []MountConfig{{Source: "/a", Target: "/b"}},
			b:    []MountConfig{{Source: "/a", Target: "/b"}},
			want: true,
		},
		{
			name: "different length",
			a:    []MountConfig{{Source: "/a", Target: "/b"}},
			b:    []MountConfig{{Source: "/a", Target: "/b"}, {Source: "/c", Target: "/d"}},
			want: false,
		},
		{
			name: "different content",
			a:    []MountConfig{{Source: "/a", Target: "/b"}},
			b:    []MountConfig{{Source: "/c", Target: "/d"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MountsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("MountsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMountConfigHasExcludes(t *testing.T) {
	tests := []struct {
		name  string
		mount MountConfig
		want  bool
	}{
		{
			name:  "no excludes",
			mount: MountConfig{Source: "/a", Target: "/b"},
			want:  false,
		},
		{
			name:  "empty excludes",
			mount: MountConfig{Source: "/a", Target: "/b", Exclude: []string{}},
			want:  false,
		},
		{
			name:  "with excludes",
			mount: MountConfig{Source: "/a", Target: "/b", Exclude: []string{"*.tmp"}},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.mount.HasExcludes()
			if got != tt.want {
				t.Errorf("MountConfig.HasExcludes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfigWithMounts(t *testing.T) {
	t.Run("simple string mounts", func(t *testing.T) {
		content := `
image = "ubuntu:latest"
mounts = ["/host:/container", "/data:/data:ro"]
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

		// Mounts[0] is workdir, user mounts follow
		if len(cfg.Mounts) != 3 {
			t.Fatalf("expected 3 mounts (workdir + 2 user), got %d", len(cfg.Mounts))
		}
		if cfg.Mounts[0].Source != "." || cfg.Mounts[0].Target != "/workspace" {
			t.Errorf("mount[0] = %v, want workdir .:/workspace", cfg.Mounts[0])
		}
		if cfg.Mounts[1].Source != "/host" || cfg.Mounts[1].Target != "/container" {
			t.Errorf("mount[1] = %v, want source=/host target=/container", cfg.Mounts[1])
		}
		if cfg.Mounts[2].Source != "/data" || cfg.Mounts[2].Target != "/data" || !cfg.Mounts[2].Readonly {
			t.Errorf("mount[2] = %v, want source=/data target=/data readonly=true", cfg.Mounts[2])
		}
	})

	t.Run("extended object mounts", func(t *testing.T) {
		content := `
image = "ubuntu:latest"
workdir = "/app"

[[mounts]]
source = "/Users/me/project"
target = "/data"
readonly = false
exclude = ["**/.env.prod", "**/secrets/"]
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

		// Mounts[0] is workdir, user mounts follow
		if len(cfg.Mounts) != 2 {
			t.Fatalf("expected 2 mounts (workdir + 1 user), got %d", len(cfg.Mounts))
		}
		if cfg.Mounts[0].Source != "." || cfg.Mounts[0].Target != "/app" {
			t.Errorf("mount[0] = %v, want workdir .:/app", cfg.Mounts[0])
		}
		m := cfg.Mounts[1]
		if m.Source != "/Users/me/project" {
			t.Errorf("mount.Source = %q, want /Users/me/project", m.Source)
		}
		if m.Target != "/data" {
			t.Errorf("mount.Target = %q, want /data", m.Target)
		}
		if m.Readonly {
			t.Error("mount.Readonly should be false")
		}
		if len(m.Exclude) != 2 {
			t.Fatalf("expected 2 excludes, got %d", len(m.Exclude))
		}
		if m.Exclude[0] != "**/.env.prod" || m.Exclude[1] != "**/secrets/" {
			t.Errorf("mount.Exclude = %v, want [**/.env.prod, **/secrets/]", m.Exclude)
		}
	})

	t.Run("multiple object mounts", func(t *testing.T) {
		// Note: TOML doesn't allow mixing inline array and array table syntax
		// for the same key. Use all object format or all string format per file.
		content := `
image = "ubuntu:latest"

[[mounts]]
source = "/simple"
target = "/simple"

[[mounts]]
source = "/extended"
target = "/extended"
exclude = ["*.tmp"]
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

		// Mounts[0] is workdir, user mounts follow
		if len(cfg.Mounts) != 3 {
			t.Fatalf("expected 3 mounts (workdir + 2 user), got %d", len(cfg.Mounts))
		}
		if !cfg.Mounts[2].HasExcludes() {
			t.Error("expected third mount (second user mount) to have excludes")
		}
	})
}

func TestRawMountSliceJSONSchema(t *testing.T) {
	schema := RawMountSlice{}.JSONSchema()

	if schema.Type != "array" {
		t.Errorf("expected type 'array', got %q", schema.Type)
	}

	if schema.Items == nil {
		t.Fatal("expected Items to be set")
	}

	if schema.Items.OneOf == nil || len(schema.Items.OneOf) != 2 {
		t.Fatalf("expected OneOf with 2 schemas, got %v", schema.Items.OneOf)
	}

	// First option should be string
	strSchema := schema.Items.OneOf[0]
	if strSchema.Type != "string" {
		t.Errorf("expected first OneOf to be string, got %q", strSchema.Type)
	}

	// Second option should be object
	objSchema := schema.Items.OneOf[1]
	if objSchema.Type != "object" {
		t.Errorf("expected second OneOf to be object, got %q", objSchema.Type)
	}

	// Check required fields
	if len(objSchema.Required) != 2 {
		t.Errorf("expected 2 required fields, got %d", len(objSchema.Required))
	}
}

func TestLoadConfigWorkdirExclude(t *testing.T) {
	t.Run("workdir_exclude normalizes into Mounts[0]", func(t *testing.T) {
		content := `
image = "ubuntu:latest"
workdir = "/app"
workdir_exclude = ["node_modules", ".git", "dist"]
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

		if len(cfg.Mounts) != 1 {
			t.Fatalf("expected 1 mount (workdir only), got %d", len(cfg.Mounts))
		}

		m := cfg.Mounts[0]
		if m.Source != "." {
			t.Errorf("workdir mount Source = %q, want \".\"", m.Source)
		}
		if m.Target != "/app" {
			t.Errorf("workdir mount Target = %q, want \"/app\"", m.Target)
		}
		if len(m.Exclude) != 3 {
			t.Fatalf("workdir mount Exclude len = %d, want 3", len(m.Exclude))
		}
		if m.Exclude[0] != "node_modules" || m.Exclude[1] != ".git" || m.Exclude[2] != "dist" {
			t.Errorf("workdir mount Exclude = %v, want [node_modules, .git, dist]", m.Exclude)
		}
	})

	t.Run("workdir without exclude normalizes with nil Exclude", func(t *testing.T) {
		content := `
image = "ubuntu:latest"
workdir = "/app"
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

		if len(cfg.Mounts) != 1 {
			t.Fatalf("expected 1 mount (workdir only), got %d", len(cfg.Mounts))
		}

		m := cfg.Mounts[0]
		if m.Source != "." || m.Target != "/app" {
			t.Errorf("workdir mount = %v, want .:/app", m)
		}
		if m.Exclude != nil {
			t.Errorf("workdir mount Exclude = %v, want nil", m.Exclude)
		}
	})

	t.Run("mount target conflicts with workdir returns error", func(t *testing.T) {
		content := `
image = "ubuntu:latest"
workdir = "/workspace"

[[mounts]]
source = "/other"
target = "/workspace"
`
		env, memFs := newTestEnv(t)
		path := "/test/.alca.toml"
		if err := afero.WriteFile(memFs, path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := LoadConfig(env, path)
		if err == nil {
			t.Fatal("expected error for mount target conflicting with workdir")
		}
		if !strings.Contains(err.Error(), "conflicts with workdir") {
			t.Errorf("expected error about workdir conflict, got: %v", err)
		}
	})

	t.Run("default workdir normalizes correctly", func(t *testing.T) {
		content := `
image = "ubuntu:latest"
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

		if len(cfg.Mounts) != 1 {
			t.Fatalf("expected 1 mount (workdir only), got %d", len(cfg.Mounts))
		}

		m := cfg.Mounts[0]
		if m.Source != "." || m.Target != "/workspace" {
			t.Errorf("workdir mount = %v, want .:/workspace (default)", m)
		}
	})
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
