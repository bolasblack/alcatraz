package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".alca.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(path)
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
	_, err := LoadConfig("/nonexistent/path/.alca.toml")
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
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".alca.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Test simple string env
	if env, ok := cfg.Envs["SIMPLE"]; !ok {
		t.Error("expected SIMPLE env to exist")
	} else if env.Value != "value1" || env.OverrideOnEnter {
		t.Errorf("SIMPLE env: got value=%q override=%v, want value='value1' override=false",
			env.Value, env.OverrideOnEnter)
	}

	// Test reference env
	if env, ok := cfg.Envs["REFERENCE"]; !ok {
		t.Error("expected REFERENCE env to exist")
	} else if env.Value != "${HOST_VAR}" {
		t.Errorf("REFERENCE env: got value=%q, want '${HOST_VAR}'", env.Value)
	}

	// Test complex object env
	if env, ok := cfg.Envs["COMPLEX"]; !ok {
		t.Error("expected COMPLEX env to exist")
	} else if env.Value != "value2" || !env.OverrideOnEnter {
		t.Errorf("COMPLEX env: got value=%q override=%v, want value='value2' override=true",
			env.Value, env.OverrideOnEnter)
	}
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
