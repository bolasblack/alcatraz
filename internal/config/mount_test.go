package config

import (
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

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

		cfg, err := LoadConfig(env, path, noExpandEnv)
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

		cfg, err := LoadConfig(env, path, noExpandEnv)
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

		cfg, err := LoadConfig(env, path, noExpandEnv)
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

// --- Mount env var expansion tests ---

func TestParseMounts_StringFormatEnvExpansion(t *testing.T) {
	expandEnv := func(s string) (string, error) {
		return strings.ReplaceAll(s, "${HOME}", "/home/user"), nil
	}

	raw := []any{"${HOME}/data:/data:ro"}
	mounts, err := parseMounts(raw, expandEnv)
	if err != nil {
		t.Fatalf("parseMounts failed: %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Source != "/home/user/data" {
		t.Errorf("expected source '/home/user/data', got %q", mounts[0].Source)
	}
	if mounts[0].Target != "/data" {
		t.Errorf("expected target '/data', got %q", mounts[0].Target)
	}
	if !mounts[0].Readonly {
		t.Error("expected readonly=true")
	}
}

func TestParseMounts_ObjectFormatEnvExpansion(t *testing.T) {
	expandEnv := func(s string) (string, error) {
		s = strings.ReplaceAll(s, "${HOST_PATH}", "/opt/shared")
		s = strings.ReplaceAll(s, "${CONTAINER_PATH}", "/mnt/shared")
		return s, nil
	}

	raw := []any{
		map[string]any{
			"source": "${HOST_PATH}/configs",
			"target": "${CONTAINER_PATH}/configs",
		},
	}
	mounts, err := parseMounts(raw, expandEnv)
	if err != nil {
		t.Fatalf("parseMounts failed: %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	if mounts[0].Source != "/opt/shared/configs" {
		t.Errorf("expected source '/opt/shared/configs', got %q", mounts[0].Source)
	}
	// Target should NOT be expanded — container paths are fixed
	if mounts[0].Target != "${CONTAINER_PATH}/configs" {
		t.Errorf("expected target '${CONTAINER_PATH}/configs' (unexpanded), got %q", mounts[0].Target)
	}
}

func TestParseMounts_StringFormatTargetNotExpanded(t *testing.T) {
	expandEnv := func(s string) (string, error) {
		s = strings.ReplaceAll(s, "${HOME}", "/home/user")
		s = strings.ReplaceAll(s, "${CONTAINER}", "/expanded")
		return s, nil
	}

	raw := []any{"${HOME}/data:${CONTAINER}/data"}
	mounts, err := parseMounts(raw, expandEnv)
	if err != nil {
		t.Fatalf("parseMounts failed: %v", err)
	}

	if len(mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(mounts))
	}
	// Source should be expanded
	if mounts[0].Source != "/home/user/data" {
		t.Errorf("expected source '/home/user/data', got %q", mounts[0].Source)
	}
	// Target should NOT be expanded — container paths are fixed
	if mounts[0].Target != "${CONTAINER}/data" {
		t.Errorf("expected target '${CONTAINER}/data' (unexpanded), got %q", mounts[0].Target)
	}
}

func TestParseMounts_NoEnvVarsUnchanged(t *testing.T) {
	raw := []any{
		"/static/path:/container/path",
		map[string]any{
			"source":   "/host/dir",
			"target":   "/container/dir",
			"readonly": true,
		},
	}
	mounts, err := parseMounts(raw, noExpandEnv)
	if err != nil {
		t.Fatalf("parseMounts failed: %v", err)
	}

	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(mounts))
	}
	if mounts[0].Source != "/static/path" || mounts[0].Target != "/container/path" {
		t.Errorf("mount[0] mismatch: got %s:%s", mounts[0].Source, mounts[0].Target)
	}
	if mounts[1].Source != "/host/dir" || mounts[1].Target != "/container/dir" || !mounts[1].Readonly {
		t.Errorf("mount[1] mismatch: got %s:%s readonly=%v", mounts[1].Source, mounts[1].Target, mounts[1].Readonly)
	}
}

func TestParseMountValue_InvalidType(t *testing.T) {
	_, err := parseMountValue(42, noExpandEnv)
	if err == nil {
		t.Fatal("expected error for invalid type, got nil")
	}
	if !strings.Contains(err.Error(), "invalid type") {
		t.Errorf("expected error containing 'invalid type', got %q", err.Error())
	}
}

func TestLoadConfig_MountEnvExpansion(t *testing.T) {
	fs := afero.NewMemMapFs()

	configContent := `
image = "test:latest"
mounts = [
  "${MY_DIR}/data:/data",
  { source = "${MY_DIR}/config", target = "/config" },
]
`
	_ = afero.WriteFile(fs, "/project/.alca.toml", []byte(configContent), 0644)

	env := &util.Env{Fs: fs, Cmd: util.NewMockCommandRunner()}

	expandEnv := func(s string) (string, error) {
		return strings.ReplaceAll(s, "${MY_DIR}", "/home/testuser"), nil
	}

	cfg, err := LoadConfig(env, "/project/.alca.toml", expandEnv)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Mounts[0] is the workdir mount, user mounts start at index 1
	if len(cfg.Mounts) != 3 {
		t.Fatalf("expected 3 mounts (1 workdir + 2 user), got %d", len(cfg.Mounts))
	}
	if cfg.Mounts[1].Source != "/home/testuser/data" {
		t.Errorf("expected mount[1] source '/home/testuser/data', got %q", cfg.Mounts[1].Source)
	}
	if cfg.Mounts[1].Target != "/data" {
		t.Errorf("expected mount[1] target '/data', got %q", cfg.Mounts[1].Target)
	}
	if cfg.Mounts[2].Source != "/home/testuser/config" {
		t.Errorf("expected mount[2] source '/home/testuser/config', got %q", cfg.Mounts[2].Source)
	}
	if cfg.Mounts[2].Target != "/config" {
		t.Errorf("expected mount[2] target '/config', got %q", cfg.Mounts[2].Target)
	}
}
