package config

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
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

	// Included file (base) wins over declaring file (main) — AGD-033
	if cfg.Image != "base:latest" {
		t.Errorf("expected image 'base:latest', got %q", cfg.Image)
	}

	// Base config's workdir is preserved (main doesn't set it)
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

	// Included files win at each level: common wins over dev, dev-result wins over main
	if cfg.Image != "common:latest" {
		t.Errorf("expected image 'common:latest', got %q", cfg.Image)
	}

	// Included file's workdir wins over main's
	if cfg.Workdir != "/common" {
		t.Errorf("expected workdir '/common', got %q", cfg.Workdir)
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

	// SHARED_VAR should be overridden by included file (base wins over main)
	if e, ok := cfg.Envs["SHARED_VAR"]; !ok || e.Value != "base_shared" {
		t.Errorf("expected SHARED_VAR='base_shared', got %v", cfg.Envs["SHARED_VAR"])
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

// --- Extends tests (AGD-033): declaring file wins over extended files ---

func TestLoadWithIncludes_SimpleExtends(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	baseContent := `
image = "base:latest"
workdir = "/base"
`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.base.toml", []byte(baseContent), 0644); err != nil {
		t.Fatalf("failed to write base file: %v", err)
	}

	mainContent := `
extends = [".alca.base.toml"]
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

	// Declaring file (main) wins over extended file (base)
	if cfg.Image != "main:latest" {
		t.Errorf("expected image 'main:latest', got %q", cfg.Image)
	}

	// Base's workdir preserved since main doesn't set it
	if cfg.Workdir != "/base" {
		t.Errorf("expected workdir '/base', got %q", cfg.Workdir)
	}
}

func TestLoadWithIncludes_NestedExtends(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	commonContent := `
image = "common:latest"
workdir = "/common"
`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.common.toml", []byte(commonContent), 0644); err != nil {
		t.Fatalf("failed to write common file: %v", err)
	}

	devContent := `
extends = [".alca.common.toml"]
image = "dev:latest"
`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.dev.toml", []byte(devContent), 0644); err != nil {
		t.Fatalf("failed to write dev file: %v", err)
	}

	mainContent := `
extends = [".alca.dev.toml"]
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

	// Each declaring file wins: main > dev > common
	if cfg.Image != "main:latest" {
		t.Errorf("expected image 'main:latest', got %q", cfg.Image)
	}

	// common's workdir inherited through chain (no override)
	if cfg.Workdir != "/common" {
		t.Errorf("expected workdir '/common', got %q", cfg.Workdir)
	}
}

func TestLoadWithIncludes_ExtendsWithGlob(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	for i, name := range []string{".alca.base-a.toml", ".alca.base-b.toml"} {
		content := `mounts = ["/mount` + string(rune('a'+i)) + `:/mount` + string(rune('a'+i)) + `"]`
		if err := afero.WriteFile(memFs, baseDir+"/"+name, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	mainContent := `
extends = [".alca.base-*.toml"]
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

	if cfg.Image != "main:latest" {
		t.Errorf("expected image 'main:latest', got %q", cfg.Image)
	}
	if len(cfg.Mounts) != 2 {
		t.Fatalf("expected 2 mounts from glob extends, got %d", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Source != "/mountb" || cfg.Mounts[0].Target != "/mountb" {
		t.Errorf("expected mount[0] /mountb:/mountb, got %s:%s", cfg.Mounts[0].Source, cfg.Mounts[0].Target)
	}
	if cfg.Mounts[1].Source != "/mounta" || cfg.Mounts[1].Target != "/mounta" {
		t.Errorf("expected mount[1] /mounta:/mounta, got %s:%s", cfg.Mounts[1].Source, cfg.Mounts[1].Target)
	}
}

func TestLoadWithIncludes_ExtendsCircularReference(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// Create circular reference via extends: a -> b -> a
	aContent := `
extends = [".alca.b.toml"]
image = "a:latest"
`
	aPath := baseDir + "/.alca.a.toml"
	if err := afero.WriteFile(memFs, aPath, []byte(aContent), 0644); err != nil {
		t.Fatalf("failed to write a file: %v", err)
	}

	bContent := `
extends = [".alca.a.toml"]
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

func TestLoadWithIncludes_MultipleIncludesAppend(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	aContent := `
[commands.up]
command = "foo"
append = true
`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.a.toml", []byte(aContent), 0644); err != nil {
		t.Fatalf("failed to write a file: %v", err)
	}

	bContent := `
[commands.up]
command = "bar"
append = true
`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.b.toml", []byte(bContent), 0644); err != nil {
		t.Fatalf("failed to write b file: %v", err)
	}

	mainContent := `
includes = [".alca.a.toml", ".alca.b.toml"]
image = "test:latest"

[commands]
up = "base"
`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// Each include appends to accumulating result: "base" + " foo" + " bar"
	if cfg.Commands.Up.Command != "base foo bar" {
		t.Errorf("expected 'base foo bar', got %q", cfg.Commands.Up.Command)
	}
}

// --- Includes new semantics tests (AGD-033): included files win ---

func TestLoadWithIncludes_IncludesOverridesImage(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	localContent := `
image = "local:latest"
`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.local.toml", []byte(localContent), 0644); err != nil {
		t.Fatalf("failed to write local file: %v", err)
	}

	mainContent := `
includes = [".alca.local.toml"]
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

	// Included file's image overrides main's
	if cfg.Image != "local:latest" {
		t.Errorf("expected image 'local:latest', got %q", cfg.Image)
	}
}

func TestLoadWithIncludes_IncludesPreservesNonOverlapping(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	localContent := `
image = "local:latest"
`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.local.toml", []byte(localContent), 0644); err != nil {
		t.Fatalf("failed to write local file: %v", err)
	}

	mainContent := `
includes = [".alca.local.toml"]
image = "main:latest"
workdir = "/main"
runtime = "docker"
`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// Included file overrides image
	if cfg.Image != "local:latest" {
		t.Errorf("expected image 'local:latest', got %q", cfg.Image)
	}
	// Fields only in main are preserved
	if cfg.Workdir != "/main" {
		t.Errorf("expected workdir '/main', got %q", cfg.Workdir)
	}
	if cfg.Runtime != RuntimeDocker {
		t.Errorf("expected runtime 'docker', got %q", cfg.Runtime)
	}
}

// --- Three-layer merge tests (AGD-033): extends + self + includes ---

func TestLoadWithIncludes_ExtendsAndIncludes(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// extends file: base layer
	extendsContent := `
image = "extends:latest"
workdir = "/extends"

[commands]
up = "extends-up"
enter = "extends-enter"
`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.base.toml", []byte(extendsContent), 0644); err != nil {
		t.Fatalf("failed to write extends file: %v", err)
	}

	// includes file: top layer (wins)
	includesContent := `
image = "includes:latest"
`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.local.toml", []byte(includesContent), 0644); err != nil {
		t.Fatalf("failed to write includes file: %v", err)
	}

	// self: middle layer
	mainContent := `
extends = [".alca.base.toml"]
includes = [".alca.local.toml"]
image = "self:latest"

[commands]
up = "self-up"
`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// Three-layer: extends(base) → self(middle) → includes(top)
	// includes wins for image
	if cfg.Image != "includes:latest" {
		t.Errorf("expected image 'includes:latest', got %q", cfg.Image)
	}

	// self wins over extends for commands.up; includes doesn't set it
	if cfg.Commands.Up.Command != "self-up" {
		t.Errorf("expected commands.up 'self-up', got %q", cfg.Commands.Up.Command)
	}

	// extends provides commands.enter (no override from self or includes)
	if cfg.Commands.Enter.Command != "extends-enter" {
		t.Errorf("expected commands.enter 'extends-enter', got %q", cfg.Commands.Enter.Command)
	}

	// extends provides workdir (no override from self or includes)
	if cfg.Workdir != "/extends" {
		t.Errorf("expected workdir '/extends', got %q", cfg.Workdir)
	}
}

// --- Array priority tests (AGD-033) ---

func TestLoadWithIncludes_ExtendsArrayPriority(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// B sets image to "b:latest"
	bContent := `image = "b:latest"`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.b.toml", []byte(bContent), 0644); err != nil {
		t.Fatalf("failed to write b file: %v", err)
	}

	// C sets image to "c:latest"
	cContent := `image = "c:latest"`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.c.toml", []byte(cContent), 0644); err != nil {
		t.Fatalf("failed to write c file: %v", err)
	}

	// Main extends [B, C] — first entry (B) should win
	mainContent := `extends = [".alca.b.toml", ".alca.c.toml"]`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// extends: first entry wins (B > C)
	if cfg.Image != "b:latest" {
		t.Errorf("expected image 'b:latest' (first entry wins), got %q", cfg.Image)
	}
}

func TestLoadWithIncludes_IncludesArrayPriority(t *testing.T) {
	env, memFs := newTestEnv(t)
	baseDir := "/test"

	// B sets image to "b:latest"
	bContent := `image = "b:latest"`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.b.toml", []byte(bContent), 0644); err != nil {
		t.Fatalf("failed to write b file: %v", err)
	}

	// C sets image to "c:latest"
	cContent := `image = "c:latest"`
	if err := afero.WriteFile(memFs, baseDir+"/.alca.c.toml", []byte(cContent), 0644); err != nil {
		t.Fatalf("failed to write c file: %v", err)
	}

	// Main includes [B, C] — last entry (C) should win
	mainContent := `includes = [".alca.b.toml", ".alca.c.toml"]`
	mainPath := baseDir + "/.alca.toml"
	if err := afero.WriteFile(memFs, mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(env, mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// includes: last entry wins (C > B)
	if cfg.Image != "c:latest" {
		t.Errorf("expected image 'c:latest' (last entry wins), got %q", cfg.Image)
	}
}

// --- Command append unit tests (AGD-034) ---

func TestMergeCommandValue_Replace(t *testing.T) {
	base := CommandValue{Command: "base-cmd"}
	overlay := CommandValue{Command: "overlay-cmd", Append: false}

	result := mergeCommandValue(base, overlay)

	if result.Command != "overlay-cmd" {
		t.Errorf("expected 'overlay-cmd', got %q", result.Command)
	}
	if result.Append {
		t.Error("expected Append=false for replace")
	}
}

func TestMergeCommandValue_Append(t *testing.T) {
	base := CommandValue{Command: "nix develop"}
	overlay := CommandValue{Command: "bash", Append: true}

	result := mergeCommandValue(base, overlay)

	if result.Command != "nix develop bash" {
		t.Errorf("expected 'nix develop bash', got %q", result.Command)
	}
	// Append flag consumed during merge
	if result.Append {
		t.Error("expected Append=false after merge")
	}
}

func TestMergeCommandValue_AppendEmptyBase(t *testing.T) {
	base := CommandValue{}
	overlay := CommandValue{Command: "bash", Append: true}

	result := mergeCommandValue(base, overlay)

	// Empty base: overlay replaces (no space-concat with empty)
	if result.Command != "bash" {
		t.Errorf("expected 'bash', got %q", result.Command)
	}
	if !result.Append {
		t.Error("expected Append=true preserved for later merges when base is empty")
	}
}

func TestMergeCommandValue_EmptyOverlay(t *testing.T) {
	base := CommandValue{Command: "base-cmd"}
	overlay := CommandValue{}

	result := mergeCommandValue(base, overlay)

	if result.Command != "base-cmd" {
		t.Errorf("expected 'base-cmd', got %q", result.Command)
	}
	if result.Append {
		t.Error("expected Append=false for empty overlay")
	}
}

func TestMergeCommandValue_BaseAppendIgnored(t *testing.T) {
	base := CommandValue{Command: "nix develop", Append: true}
	overlay := CommandValue{Command: "bash", Append: false}

	result := mergeCommandValue(base, overlay)

	// Base's append flag is ignored; overlay has append=false → replace
	if result.Command != "bash" {
		t.Errorf("expected 'bash', got %q", result.Command)
	}
	if result.Append {
		t.Error("expected Append=false when base append is ignored")
	}
}

// --- parseCommandValue tests (AGD-034) ---

func TestParseCommandValue_String(t *testing.T) {
	cv, err := parseCommandValue("docker compose up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cv.Command != "docker compose up" {
		t.Errorf("expected command 'docker compose up', got %q", cv.Command)
	}
	if cv.Append {
		t.Error("expected Append=false for string input")
	}
}

func TestParseCommandValue_Struct(t *testing.T) {
	cv, err := parseCommandValue(map[string]any{"command": "bash", "append": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cv.Command != "bash" {
		t.Errorf("expected command 'bash', got %q", cv.Command)
	}
	if !cv.Append {
		t.Error("expected Append=true")
	}
}

func TestParseCommandValue_StructNoAppend(t *testing.T) {
	cv, err := parseCommandValue(map[string]any{"command": "bash"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cv.Command != "bash" {
		t.Errorf("expected command 'bash', got %q", cv.Command)
	}
	if cv.Append {
		t.Error("expected Append=false when not specified")
	}
}

func TestParseCommandValue_Nil(t *testing.T) {
	cv, err := parseCommandValue(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cv.Command != "" {
		t.Errorf("expected empty command, got %q", cv.Command)
	}
}

func TestParseCommandValue_InvalidType(t *testing.T) {
	_, err := parseCommandValue(123)
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestCommandValue_UnmarshalJSON_String(t *testing.T) {
	// Backward compat: old state.json stored commands as plain strings
	var cv CommandValue
	if err := json.Unmarshal([]byte(`"apt update"`), &cv); err != nil {
		t.Fatalf("UnmarshalJSON string failed: %v", err)
	}
	if cv.Command != "apt update" {
		t.Errorf("expected command 'apt update', got %q", cv.Command)
	}
	if cv.Append {
		t.Error("expected Append=false for string format")
	}
}

func TestCommandValue_UnmarshalJSON_Object(t *testing.T) {
	var cv CommandValue
	if err := json.Unmarshal([]byte(`{"command":"bash","append":true}`), &cv); err != nil {
		t.Fatalf("UnmarshalJSON object failed: %v", err)
	}
	if cv.Command != "bash" {
		t.Errorf("expected command 'bash', got %q", cv.Command)
	}
	if !cv.Append {
		t.Error("expected Append=true")
	}
}

// --- Integration tests: command append with includes/extends (AGD-034) ---

func TestLoadConfig_CommandAppendExample(t *testing.T) {
	fs := afero.NewMemMapFs()

	mainConfig := `
image = "test:latest"
includes = ["./*.local.toml"]

[commands.up]
command = "nix develop"
`
	localConfig := `
[commands.up]
command = "bash"
append = true
`
	if err := afero.WriteFile(fs, "/project/.alca.toml", []byte(mainConfig), 0644); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}
	if err := afero.WriteFile(fs, "/project/.alca.local.toml", []byte(localConfig), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	env := &util.Env{Fs: fs, Cmd: util.NewMockCommandRunner()}

	cfg, err := LoadConfig(env, "/project/.alca.toml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// includes: local is overlay, local has append=true → "nix develop" + " " + "bash"
	if cfg.Commands.Up.Command != "nix develop bash" {
		t.Errorf("expected 'nix develop bash', got %q", cfg.Commands.Up.Command)
	}
}

func TestLoadConfig_CommandAppendWithExtends(t *testing.T) {
	fs := afero.NewMemMapFs()

	baseConfig := `
image = "test:latest"

[commands.up]
command = "nix develop"
`
	mainConfig := `
extends = ["base.toml"]

[commands.up]
command = "bash"
append = true
`
	if err := afero.WriteFile(fs, "/project/base.toml", []byte(baseConfig), 0644); err != nil {
		t.Fatalf("failed to write base config: %v", err)
	}
	if err := afero.WriteFile(fs, "/project/.alca.toml", []byte(mainConfig), 0644); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	env := &util.Env{Fs: fs, Cmd: util.NewMockCommandRunner()}

	cfg, err := LoadConfig(env, "/project/.alca.toml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// extends: main is overlay over base, main has append=true → "nix develop" + " " + "bash"
	if cfg.Commands.Up.Command != "nix develop bash" {
		t.Errorf("expected 'nix develop bash', got %q", cfg.Commands.Up.Command)
	}
}

func TestLoadConfig_CommandAppendBaseIgnored(t *testing.T) {
	fs := afero.NewMemMapFs()

	baseConfig := `
image = "test:latest"

[commands.up]
command = "nix develop"
append = true
`
	mainConfig := `
extends = ["base.toml"]

[commands.up]
command = "bash"
`
	if err := afero.WriteFile(fs, "/project/base.toml", []byte(baseConfig), 0644); err != nil {
		t.Fatalf("failed to write base config: %v", err)
	}
	if err := afero.WriteFile(fs, "/project/.alca.toml", []byte(mainConfig), 0644); err != nil {
		t.Fatalf("failed to write main config: %v", err)
	}

	env := &util.Env{Fs: fs, Cmd: util.NewMockCommandRunner()}

	cfg, err := LoadConfig(env, "/project/.alca.toml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// extends: main is overlay, main has append=false → replace, base's append ignored
	if cfg.Commands.Up.Command != "bash" {
		t.Errorf("expected 'bash', got %q", cfg.Commands.Up.Command)
	}
}

func TestConfigToRaw(t *testing.T) {
	cfg := Config{
		Image:   "test:latest",
		Workdir: "/test",
		Runtime: RuntimeDocker,
		Commands: Commands{
			Up:    CommandValue{Command: "up cmd"},
			Enter: CommandValue{Command: "enter cmd"},
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
	if raw.Commands.Up != cfg.Commands.Up.Command {
		t.Errorf("Commands.Up mismatch: got %q, want %q", raw.Commands.Up, cfg.Commands.Up.Command)
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
			Up:    CommandValue{Command: "base up"},
			Enter: CommandValue{Command: "base enter"},
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
			Up: CommandValue{Command: "overlay up"},
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
	if result.Commands.Up.Command != "overlay up" {
		t.Errorf("expected commands.up 'overlay up', got %q", result.Commands.Up.Command)
	}
	if result.Commands.Enter.Command != "base enter" {
		t.Errorf("expected commands.enter 'base enter', got %q", result.Commands.Enter.Command)
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

// TestMergeConfigs_LANAccessEmptyOverlay verifies that when overlay has
// an explicit empty slice for Network.LANAccess, the base value is preserved.
// This tests the user scenario: main config has lan-access=[], included config has rules.
func TestMergeConfigs_LANAccessEmptyOverlay(t *testing.T) {
	base := Config{
		Network: Network{
			LANAccess: []string{"192.168.1.2:10000"},
		},
	}

	// Explicit empty slice (not nil) - simulates TOML parsing of "lan-access = []"
	overlay := Config{
		Network: Network{
			LANAccess: []string{},
		},
	}

	result := mergeConfigs(base, overlay)

	// Base LANAccess should be preserved when overlay is empty
	if len(result.Network.LANAccess) != 1 {
		t.Errorf("expected 1 LANAccess rule, got %d", len(result.Network.LANAccess))
	}
	if len(result.Network.LANAccess) > 0 && result.Network.LANAccess[0] != "192.168.1.2:10000" {
		t.Errorf("expected LANAccess[0]='192.168.1.2:10000', got %q", result.Network.LANAccess[0])
	}
}

// TestLoadConfig_LANAccessFromInclude tests the full TOML loading path:
// main config has empty lan-access=[], included config has rules.
// This is an integration test matching the real user scenario.
func TestLoadConfig_LANAccessFromInclude(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Create main config with empty lan-access and includes
	mainConfig := `
image = "test:latest"
includes = ["./*.local.toml"]

[network]
lan-access = []
`
	// Create local config with lan-access rule
	localConfig := `
[network]
lan-access = ["192.168.1.2:10000"]
`

	_ = afero.WriteFile(fs, "/project/.alca.toml", []byte(mainConfig), 0644)
	_ = afero.WriteFile(fs, "/project/.alca.local.toml", []byte(localConfig), 0644)

	env := &util.Env{Fs: fs, Cmd: util.NewMockCommandRunner()}

	cfg, err := LoadConfig(env, "/project/.alca.toml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// The included lan-access rule should be present
	if len(cfg.Network.LANAccess) != 1 {
		t.Errorf("expected 1 LANAccess rule from include, got %d: %v", len(cfg.Network.LANAccess), cfg.Network.LANAccess)
	}
	if len(cfg.Network.LANAccess) > 0 && cfg.Network.LANAccess[0] != "192.168.1.2:10000" {
		t.Errorf("expected LANAccess[0]='192.168.1.2:10000', got %q", cfg.Network.LANAccess[0])
	}
}

// TestLoadConfig_CapsArrayMode tests caps parsing in array mode: caps = ["SETUID", "SETGID"]
func TestLoadConfig_CapsArrayMode(t *testing.T) {
	fs := afero.NewMemMapFs()

	config := `
image = "test:latest"
caps = ["SETUID", "SETGID"]
`
	_ = afero.WriteFile(fs, "/project/.alca.toml", []byte(config), 0644)

	env := &util.Env{Fs: fs, Cmd: util.NewMockCommandRunner()}

	cfg, err := LoadConfig(env, "/project/.alca.toml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Array mode should drop ALL and add defaults + user caps
	expectedDrop := []string{"ALL"}
	expectedAdd := []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "KILL", "SETUID", "SETGID"}

	if len(cfg.Caps.Drop) != len(expectedDrop) {
		t.Errorf("expected Drop %v, got %v", expectedDrop, cfg.Caps.Drop)
	}
	if len(cfg.Caps.Add) != len(expectedAdd) {
		t.Errorf("expected Add %v, got %v", expectedAdd, cfg.Caps.Add)
	}

	// Verify all expected caps are present
	for _, expected := range expectedAdd {
		found := false
		for _, actual := range cfg.Caps.Add {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected cap %q in Add, got %v", expected, cfg.Caps.Add)
		}
	}
}

// TestLoadConfig_CapsObjectMode tests caps parsing in object mode: [caps] add = [...] drop = [...]
func TestLoadConfig_CapsObjectMode(t *testing.T) {
	fs := afero.NewMemMapFs()

	config := `
image = "test:latest"

[caps]
drop = ["NET_RAW"]
add = ["SYS_ADMIN"]
`
	_ = afero.WriteFile(fs, "/project/.alca.toml", []byte(config), 0644)

	env := &util.Env{Fs: fs, Cmd: util.NewMockCommandRunner()}

	cfg, err := LoadConfig(env, "/project/.alca.toml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Object mode uses explicit values without defaults
	if len(cfg.Caps.Drop) != 1 || cfg.Caps.Drop[0] != "NET_RAW" {
		t.Errorf("expected Drop [NET_RAW], got %v", cfg.Caps.Drop)
	}
	if len(cfg.Caps.Add) != 1 || cfg.Caps.Add[0] != "SYS_ADMIN" {
		t.Errorf("expected Add [SYS_ADMIN], got %v", cfg.Caps.Add)
	}
}

// TestLoadConfig_CapsInlineTableMode tests caps parsing in inline table mode: caps = { add = [...] }
func TestLoadConfig_CapsInlineTableMode(t *testing.T) {
	fs := afero.NewMemMapFs()

	config := `
image = "test:latest"
caps = { add = ["CHOWN", "FOWNER"] }
`
	_ = afero.WriteFile(fs, "/project/.alca.toml", []byte(config), 0644)

	env := &util.Env{Fs: fs, Cmd: util.NewMockCommandRunner()}

	cfg, err := LoadConfig(env, "/project/.alca.toml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Inline table mode uses explicit values
	if len(cfg.Caps.Drop) != 0 {
		t.Errorf("expected empty Drop, got %v", cfg.Caps.Drop)
	}
	if len(cfg.Caps.Add) != 2 {
		t.Errorf("expected 2 Add caps, got %v", cfg.Caps.Add)
	}
}

// TestLoadConfig_CapsEmpty tests that empty caps uses defaults
func TestLoadConfig_CapsEmpty(t *testing.T) {
	fs := afero.NewMemMapFs()

	config := `
image = "test:latest"
`
	_ = afero.WriteFile(fs, "/project/.alca.toml", []byte(config), 0644)

	env := &util.Env{Fs: fs, Cmd: util.NewMockCommandRunner()}

	cfg, err := LoadConfig(env, "/project/.alca.toml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// No caps field means apply secure defaults
	expectedDrop := []string{"ALL"}
	expectedAdd := []string{"CHOWN", "DAC_OVERRIDE", "FOWNER", "KILL", "SETUID", "SETGID"}

	if len(cfg.Caps.Drop) != len(expectedDrop) {
		t.Errorf("expected Drop %v, got %v", expectedDrop, cfg.Caps.Drop)
	}
	if len(cfg.Caps.Add) != len(expectedAdd) {
		t.Errorf("expected Add %v, got %v", expectedAdd, cfg.Caps.Add)
	}
}
