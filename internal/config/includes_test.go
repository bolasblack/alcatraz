package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWithIncludes_SimpleInclude(t *testing.T) {
	tmpDir := t.TempDir()

	// Create base config
	baseContent := `
image = "base:latest"
workdir = "/base"
`
	basePath := filepath.Join(tmpDir, ".alca.base.toml")
	if err := os.WriteFile(basePath, []byte(baseContent), 0644); err != nil {
		t.Fatalf("failed to write base file: %v", err)
	}

	// Create main config that includes base
	mainContent := `
includes = [".alca.base.toml"]
image = "main:latest"
`
	mainPath := filepath.Join(tmpDir, ".alca.toml")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(mainPath)
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
	tmpDir := t.TempDir()

	// Create common config (innermost)
	commonContent := `
image = "common:latest"
workdir = "/common"
mounts = ["/common:/common"]
`
	commonPath := filepath.Join(tmpDir, ".alca.common.toml")
	if err := os.WriteFile(commonPath, []byte(commonContent), 0644); err != nil {
		t.Fatalf("failed to write common file: %v", err)
	}

	// Create dev config that includes common
	devContent := `
includes = [".alca.common.toml"]
image = "dev:latest"
mounts = ["/dev:/dev"]
`
	devPath := filepath.Join(tmpDir, ".alca.dev.toml")
	if err := os.WriteFile(devPath, []byte(devContent), 0644); err != nil {
		t.Fatalf("failed to write dev file: %v", err)
	}

	// Create main config that includes dev
	mainContent := `
includes = [".alca.dev.toml"]
workdir = "/main"
`
	mainPath := filepath.Join(tmpDir, ".alca.toml")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(mainPath)
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
	tmpDir := t.TempDir()

	// Create multiple include files matching glob pattern
	for i, name := range []string{".alca.a.toml", ".alca.b.toml", ".alca.c.toml"} {
		content := `mounts = ["/mount` + string(rune('a'+i)) + `:/mount` + string(rune('a'+i)) + `"]`
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	// Create main config with glob pattern
	mainContent := `
includes = [".alca.*.toml"]
image = "main:latest"
`
	mainPath := filepath.Join(tmpDir, ".alca.toml")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// All 3 glob-matched files should have their mounts included
	if len(cfg.Mounts) != 3 {
		t.Errorf("expected 3 mounts from glob, got %d: %v", len(cfg.Mounts), cfg.Mounts)
	}
}

func TestLoadWithIncludes_CircularReference(t *testing.T) {
	tmpDir := t.TempDir()

	// Create circular reference: a -> b -> a
	aContent := `
includes = [".alca.b.toml"]
image = "a:latest"
`
	aPath := filepath.Join(tmpDir, ".alca.a.toml")
	if err := os.WriteFile(aPath, []byte(aContent), 0644); err != nil {
		t.Fatalf("failed to write a file: %v", err)
	}

	bContent := `
includes = [".alca.a.toml"]
image = "b:latest"
`
	bPath := filepath.Join(tmpDir, ".alca.b.toml")
	if err := os.WriteFile(bPath, []byte(bContent), 0644); err != nil {
		t.Fatalf("failed to write b file: %v", err)
	}

	_, err := LoadWithIncludes(aPath)
	if err == nil {
		t.Error("expected circular reference error, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected error to mention 'circular', got: %v", err)
	}
}

func TestLoadWithIncludes_NestedCircularReference(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested circular reference: a -> b -> c -> a
	aContent := `
includes = [".alca.b.toml"]
image = "a:latest"
`
	aPath := filepath.Join(tmpDir, ".alca.a.toml")
	if err := os.WriteFile(aPath, []byte(aContent), 0644); err != nil {
		t.Fatalf("failed to write a file: %v", err)
	}

	bContent := `
includes = [".alca.c.toml"]
image = "b:latest"
`
	bPath := filepath.Join(tmpDir, ".alca.b.toml")
	if err := os.WriteFile(bPath, []byte(bContent), 0644); err != nil {
		t.Fatalf("failed to write b file: %v", err)
	}

	cContent := `
includes = [".alca.a.toml"]
image = "c:latest"
`
	cPath := filepath.Join(tmpDir, ".alca.c.toml")
	if err := os.WriteFile(cPath, []byte(cContent), 0644); err != nil {
		t.Fatalf("failed to write c file: %v", err)
	}

	_, err := LoadWithIncludes(aPath)
	if err == nil {
		t.Error("expected circular reference error for nested cycle, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected error to mention 'circular', got: %v", err)
	}
}

func TestLoadWithIncludes_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create main config that includes a non-existent file
	mainContent := `
includes = [".alca.nonexistent.toml"]
image = "main:latest"
`
	mainPath := filepath.Join(tmpDir, ".alca.toml")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	_, err := LoadWithIncludes(mainPath)
	if err == nil {
		t.Error("expected error for missing include file, got nil")
	}
	// Verify error message contains the file path
	if !strings.Contains(err.Error(), ".alca.nonexistent.toml") {
		t.Errorf("expected error to contain file path '.alca.nonexistent.toml', got: %v", err)
	}
}

func TestLoadWithIncludes_EmptyGlob(t *testing.T) {
	tmpDir := t.TempDir()

	// Create main config with glob that matches nothing
	mainContent := `
includes = [".alca.nonexistent.*.toml"]
image = "main:latest"
`
	mainPath := filepath.Join(tmpDir, ".alca.toml")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	// Empty glob is OK - should not error
	cfg, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("expected empty glob to succeed, got error: %v", err)
	}

	if cfg.Image != "main:latest" {
		t.Errorf("expected image 'main:latest', got %q", cfg.Image)
	}
}

func TestLoadWithIncludes_EnvsMerge(t *testing.T) {
	tmpDir := t.TempDir()

	// Create base config with some envs
	baseContent := `
image = "base:latest"

[envs]
BASE_VAR = "base_value"
SHARED_VAR = "base_shared"
`
	basePath := filepath.Join(tmpDir, ".alca.base.toml")
	if err := os.WriteFile(basePath, []byte(baseContent), 0644); err != nil {
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
	mainPath := filepath.Join(tmpDir, ".alca.toml")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	cfg, err := LoadWithIncludes(mainPath)
	if err != nil {
		t.Fatalf("LoadWithIncludes failed: %v", err)
	}

	// BASE_VAR should be preserved from base
	if env, ok := cfg.Envs["BASE_VAR"]; !ok || env.Value != "base_value" {
		t.Errorf("expected BASE_VAR='base_value', got %v", cfg.Envs["BASE_VAR"])
	}

	// MAIN_VAR should be from main
	if env, ok := cfg.Envs["MAIN_VAR"]; !ok || env.Value != "main_value" {
		t.Errorf("expected MAIN_VAR='main_value', got %v", cfg.Envs["MAIN_VAR"])
	}

	// SHARED_VAR should be overridden by main
	if env, ok := cfg.Envs["SHARED_VAR"]; !ok || env.Value != "main_shared" {
		t.Errorf("expected SHARED_VAR='main_shared', got %v", cfg.Envs["SHARED_VAR"])
	}
}

func TestMergeConfigs(t *testing.T) {
	base := Config{
		Image:   "base:latest",
		Workdir: "/base",
		Mounts:  []string{"/base:/base"},
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
		Mounts: []string{"/overlay:/overlay"},
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
