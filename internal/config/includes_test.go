package config

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestLoadWithIncludes_SimpleInclude(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create base config
	baseContent := `
image = "base:latest"
workdir = "/base"
`
	basePath := baseDir + "/.alca.base.toml"
	if err := afero.WriteFile(memFs, basePath, []byte(baseContent), 0644); err != nil {
		t.Fatalf("failed to write base file: %v", err)
	}

	// Create main config that includes base
	mainContent := `
includes = [".alca.base.toml"]
image = "main:latest"
`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// Main config overrides base config's image
	if cfg.Image != "main:latest" {
		t.Errorf("expected image 'main:latest', got %q", cfg.Image)
	}

	// Base config's workdir is preserved since main doesn't override it
	if cfg.Workdir != "/base" {
		t.Errorf("expected workdir '/base', got %q", cfg.Workdir)
	}
}

func TestLoadWithIncludes_NestedInclude(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create common config (innermost)
	commonContent := `
image = "common:latest"
workdir = "/common"
mounts = ["/common:/common"]
`
	commonPath := baseDir + "/.alca.common.toml"
	if err := afero.WriteFile(memFs, commonPath, []byte(commonContent), 0644); err != nil {
		t.Fatalf("failed to write common file: %v", err)
	}

	// Create dev config that includes common
	devContent := `
includes = [".alca.common.toml"]
image = "dev:latest"
mounts = ["/dev:/dev"]
`
	devPath := baseDir + "/.alca.dev.toml"
	if err := afero.WriteFile(memFs, devPath, []byte(devContent), 0644); err != nil {
		t.Fatalf("failed to write dev file: %v", err)
	}

	// Create main config that includes dev
	mainContent := `
includes = [".alca.dev.toml"]
workdir = "/main"
`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// Dev config's image (overrode common)
	if cfg.Image != "dev:latest" {
		t.Errorf("expected image 'dev:latest', got %q", cfg.Image)
	}

	// Main config's workdir (overrode common)
	if cfg.Workdir != "/main" {
		t.Errorf("expected workdir '/main', got %q", cfg.Workdir)
	}

	// Mounts should be concatenated: common + dev
	if len(cfg.Mounts) != 2 {
		t.Errorf("expected 2 mounts, got %d: %v", len(cfg.Mounts), cfg.Mounts)
	}
}

func TestLoadWithIncludes_GlobPattern(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create multiple include files matching glob pattern
	for i, name := range []string{".alca.a.toml", ".alca.b.toml", ".alca.c.toml"} {
		content := `mounts = ["/mount` + string(rune('a'+i)) + `:/mount` + string(rune('a'+i)) + `"]`
		if err := afero.WriteFile(memFs, baseDir+"/"+name, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	// Create main config with glob pattern
	mainContent := `
includes = [".alca.*.toml"]
image = "main:latest"
`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// All 3 glob-matched files should have their mounts included
	if len(cfg.Mounts) != 3 {
		t.Errorf("expected 3 mounts from glob, got %d: %v", len(cfg.Mounts), cfg.Mounts)
	}
}

func TestLoadWithIncludes_CircularReference(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create circular reference: a -> b -> a
	aContent := `
includes = [".alca.b.toml"]
image = "a:latest"
`
	aPath := baseDir + "/.alca.a.toml"
	if err := afero.WriteFile(memFs, aPath, []byte(aContent), 0644); err != nil {
		t.Fatalf("failed to write a file: %v", err)
	}

	bContent := `
includes = [".alca.a.toml"]
image = "b:latest"
`
	bPath := baseDir + "/.alca.b.toml"
	if err := afero.WriteFile(memFs, bPath, []byte(bContent), 0644); err != nil {
		t.Fatalf("failed to write b file: %v", err)
	}

	_, err := LoadWithIncludes(env, aPath)
	if err == nil {
		t.Error("expected circular reference error, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected error to mention 'circular', got: %v", err)
	}
}

func TestLoadWithIncludes_NestedCircularReference(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create nested circular reference: a -> b -> c -> a
	aContent := `
includes = [".alca.b.toml"]
image = "a:latest"
`
	aPath := baseDir + "/.alca.a.toml"
	if err := afero.WriteFile(memFs, aPath, []byte(aContent), 0644); err != nil {
		t.Fatalf("failed to write a file: %v", err)
	}

	bContent := `
includes = [".alca.c.toml"]
image = "b:latest"
`
	bPath := baseDir + "/.alca.b.toml"
	if err := afero.WriteFile(memFs, bPath, []byte(bContent), 0644); err != nil {
		t.Fatalf("failed to write b file: %v", err)
	}

	cContent := `
includes = [".alca.a.toml"]
image = "c:latest"
`
	cPath := baseDir + "/.alca.c.toml"
	if err := afero.WriteFile(memFs, cPath, []byte(cContent), 0644); err != nil {
		t.Fatalf("failed to write c file: %v", err)
	}

	_, err := LoadWithIncludes(env, aPath)
	if err == nil {
		t.Error("expected circular reference error for nested cycle, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected error to mention 'circular', got: %v", err)
	}
}

func TestLoadWithIncludes_MissingFile(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create main config that includes a non-existent file
	mainContent := `
includes = [".alca.nonexistent.toml"]
image = "main:latest"
`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	_, err := LoadWithIncludes(env, mainPath)
	if err == nil {
		t.Error("expected error for missing include file, got nil")
	}
	// Verify error message contains the file path
	if !strings.Contains(err.Error(), ".alca.nonexistent.toml") {
		t.Errorf("expected error to contain file path '.alca.nonexistent.toml', got: %v", err)
	}
}

func TestLoadWithIncludes_EmptyGlob(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create main config with glob that matches nothing
	mainContent := `
includes = [".alca.nonexistent.*.toml"]
image = "main:latest"
`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	// Empty glob is OK - should not error
	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("expected empty glob to succeed, got error: %v", err)
	}

	if cfg.Image != "main:latest" {
		t.Errorf("expected image 'main:latest', got %q", cfg.Image)
	}
}

func TestLoadWithIncludes_EnvsMerge(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create base config with some envs
	baseContent := `
image = "base:latest"

[envs]
BASE_VAR = "base_value"
SHARED_VAR = "base_shared"
`
	basePath := baseDir + "/.alca.base.toml"
	if err := afero.WriteFile(memFs, basePath, []byte(baseContent), 0644); err != nil {
		t.Fatalf("failed to write base file: %v", err)
	}

	// Create main config that overrides one env
	mainContent := `
includes = [".alca.base.toml"]
image = "main:latest"

[envs]
MAIN_VAR = "main_value"
SHARED_VAR = "main_shared"
`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// BASE_VAR should be preserved from base
	if e, ok := cfg.Envs["BASE_VAR"]; !ok || e.Value != "base_value" {
		t.Errorf("expected BASE_VAR='base_value', got %v", cfg.Envs["BASE_VAR"])
	}

	// MAIN_VAR should be from main
	if e, ok := cfg.Envs["MAIN_VAR"]; !ok || e.Value != "main_value" {
		t.Errorf("expected MAIN_VAR='main_value', got %v", cfg.Envs["MAIN_VAR"])
	}

	// SHARED_VAR should be overridden by main
	if e, ok := cfg.Envs["SHARED_VAR"]; !ok || e.Value != "main_shared" {
		t.Errorf("expected SHARED_VAR='main_shared', got %v", cfg.Envs["SHARED_VAR"])
	}
}

func TestParseEnvValue(t *testing.T) {
	tests := []struct {
		name         string
		input        any
		wantValue    string
		wantOverride bool
		wantErr      bool
	}{
		{
			name:         "string value",
			input:        "hello",
			wantValue:    "hello",
			wantOverride: false,
		},
		{
			name:         "map with value only",
			input:        map[string]any{"value": "world"},
			wantValue:    "world",
			wantOverride: false,
		},
		{
			name:         "map with value and override",
			input:        map[string]any{"value": "test", "override_on_enter": true},
			wantValue:    "test",
			wantOverride: true,
		},
		{
			name:         "empty map",
			input:        map[string]any{},
			wantValue:    "",
			wantOverride: false,
		},
		{
			name:    "invalid type int",
			input:   123,
			wantErr: true,
		},
		{
			name:    "invalid type slice",
			input:   []string{"a", "b"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEnvValue(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseEnvValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.Value != tt.wantValue {
				t.Errorf("parseEnvValue().Value = %q, want %q", got.Value, tt.wantValue)
			}
			if got.OverrideOnEnter != tt.wantOverride {
				t.Errorf("parseEnvValue().OverrideOnEnter = %v, want %v", got.OverrideOnEnter, tt.wantOverride)
			}
		})
	}
}

func TestIsGlobPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		{"file.toml", false},
		{"/path/to/file.toml", false},
		{"*.toml", true},
		{"file?.toml", true},
		{"[abc].toml", true},
		{".alca.*.toml", true},
		{"**/*.toml", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := isGlobPattern(tt.pattern)
			if got != tt.want {
				t.Errorf("isGlobPattern(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestExpandGlob(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create test files
	files := []string{"a.toml", "b.toml", "c.txt"}
	for _, f := range files {
		path := baseDir + "/" + f
		if err := afero.WriteFile(memFs, path, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	t.Run("literal path exists", func(t *testing.T) {
		path := baseDir + "/a.toml"
		matches, err := expandGlob(env, path)
		if err != nil {
			t.Fatalf("expandGlob() error = %v", err)
		}
		if len(matches) != 1 || matches[0] != path {
			t.Errorf("expandGlob() = %v, want [%s]", matches, path)
		}
	})

	t.Run("literal path not exists", func(t *testing.T) {
		path := baseDir + "/nonexistent.toml"
		_, err := expandGlob(env, path)
		if err == nil {
			t.Error("expected error for nonexistent literal path")
		}
	})

	t.Run("glob pattern matches", func(t *testing.T) {
		pattern := baseDir + "/*.toml"
		matches, err := expandGlob(env, pattern)
		if err != nil {
			t.Fatalf("expandGlob() error = %v", err)
		}
		if len(matches) != 2 {
			t.Errorf("expected 2 matches, got %d: %v", len(matches), matches)
		}
	})

	t.Run("glob pattern no matches", func(t *testing.T) {
		pattern := baseDir + "/*.json"
		matches, err := expandGlob(env, pattern)
		if err != nil {
			t.Fatalf("expandGlob() error = %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("expected 0 matches, got %d: %v", len(matches), matches)
		}
	})

	t.Run("results are sorted", func(t *testing.T) {
		pattern := baseDir + "/*.toml"
		matches, err := expandGlob(env, pattern)
		if err != nil {
			t.Fatalf("expandGlob() error = %v", err)
		}
		for i := 1; i < len(matches); i++ {
			if matches[i] < matches[i-1] {
				t.Errorf("matches not sorted: %v", matches)
			}
		}
	})
}

func TestConfigToRaw(t *testing.T) {
	cfg := Config{
		Image:   "test:latest",
		Workdir: "/test",
		Runtime: RuntimeDocker,
		Commands: Commands{
			Up:    "up cmd",
			Enter: "enter cmd",
		},
		Mounts: []MountConfig{
			{Source: "/a", Target: "/a"},
			{Source: "/b", Target: "/b"},
		},
		Resources: Resources{
			Memory: "4g",
			CPUs:   2,
		},
		Envs: map[string]EnvValue{
			"KEY1": {Value: "value1"},
			"KEY2": {Value: "${VAR}", OverrideOnEnter: true},
		},
		Network: Network{
			LANAccess: []string{"*"},
		},
	}

	raw := configToRaw(cfg)

	if raw.Image != cfg.Image {
		t.Errorf("Image mismatch: got %q, want %q", raw.Image, cfg.Image)
	}
	if raw.Workdir != cfg.Workdir {
		t.Errorf("Workdir mismatch: got %q, want %q", raw.Workdir, cfg.Workdir)
	}
	if raw.Runtime != cfg.Runtime {
		t.Errorf("Runtime mismatch: got %q, want %q", raw.Runtime, cfg.Runtime)
	}
	if raw.Commands.Up != cfg.Commands.Up {
		t.Errorf("Commands.Up mismatch: got %q, want %q", raw.Commands.Up, cfg.Commands.Up)
	}
	if len(raw.Mounts) != len(cfg.Mounts) {
		t.Errorf("Mounts length mismatch: got %d, want %d", len(raw.Mounts), len(cfg.Mounts))
	}
	if raw.Resources.Memory != cfg.Resources.Memory {
		t.Errorf("Resources.Memory mismatch: got %q, want %q", raw.Resources.Memory, cfg.Resources.Memory)
	}
	if len(raw.Envs) != len(cfg.Envs) {
		t.Errorf("Envs length mismatch: got %d, want %d", len(raw.Envs), len(cfg.Envs))
	}
	if len(raw.Network.LANAccess) != len(cfg.Network.LANAccess) {
		t.Errorf("Network.LANAccess length mismatch: got %d, want %d", len(raw.Network.LANAccess), len(cfg.Network.LANAccess))
	}
}

func TestConfigToRawEmptyEnvs(t *testing.T) {
	cfg := Config{
		Image: "test:latest",
		Envs:  nil,
	}

	raw := configToRaw(cfg)

	if raw.Envs != nil {
		t.Errorf("expected nil Envs for empty config, got %v", raw.Envs)
	}
}

func TestMergeConfigs(t *testing.T) {
	base := Config{
		Image:   "base:latest",
		Workdir: "/base",
		Mounts:  []MountConfig{{Source: "/base", Target: "/base"}},
		Commands: Commands{
			Up:    "base up",
			Enter: "base enter",
		},
		Resources: Resources{
			Memory: "2g",
			CPUs:   2,
		},
		Envs: map[string]EnvValue{
			"BASE_KEY": {Value: "base_value"},
		},
	}

	overlay := Config{
		Image:  "overlay:latest",
		Mounts: []MountConfig{{Source: "/overlay", Target: "/overlay"}},
		Commands: Commands{
			Up: "overlay up",
		},
		Resources: Resources{
			Memory: "4g",
		},
		Envs: map[string]EnvValue{
			"OVERLAY_KEY": {Value: "overlay_value"},
		},
	}

	result := mergeConfigs(base, overlay)

	// Overlay wins for simple fields
	if result.Image != "overlay:latest" {
		t.Errorf("expected image 'overlay:latest', got %q", result.Image)
	}

	// Base preserved when overlay is empty
	if result.Workdir != "/base" {
		t.Errorf("expected workdir '/base', got %q", result.Workdir)
	}

	// Arrays are concatenated
	if len(result.Mounts) != 2 {
		t.Errorf("expected 2 mounts, got %d", len(result.Mounts))
	}

	// Commands deep merged
	if result.Commands.Up != "overlay up" {
		t.Errorf("expected commands.up 'overlay up', got %q", result.Commands.Up)
	}
	if result.Commands.Enter != "base enter" {
		t.Errorf("expected commands.enter 'base enter', got %q", result.Commands.Enter)
	}

	// Resources deep merged
	if result.Resources.Memory != "4g" {
		t.Errorf("expected resources.memory '4g', got %q", result.Resources.Memory)
	}
	if result.Resources.CPUs != 2 {
		t.Errorf("expected resources.cpus 2, got %d", result.Resources.CPUs)
	}

	// Envs merged
	if len(result.Envs) != 2 {
		t.Errorf("expected 2 envs, got %d", len(result.Envs))
	}
}
