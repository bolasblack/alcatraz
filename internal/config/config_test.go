package config

import (
	"os"
	"path/filepath"
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

func TestSaveConfig(t *testing.T) {
	cfg := Config{
		Image:   "alpine:latest",
		Workdir: "/home",
		Commands: Commands{
			Up:    "apk update",
			Enter: "sh",
		},
		Mounts: []string{"/src:/dst"},
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, ".alca.toml")

	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig after save failed: %v", err)
	}

	if loaded.Image != cfg.Image {
		t.Errorf("image mismatch: expected %q, got %q", cfg.Image, loaded.Image)
	}
	if loaded.Workdir != cfg.Workdir {
		t.Errorf("workdir mismatch: expected %q, got %q", cfg.Workdir, loaded.Workdir)
	}
	if loaded.Commands.Up != cfg.Commands.Up {
		t.Errorf("commands.up mismatch: expected %q, got %q", cfg.Commands.Up, loaded.Commands.Up)
	}
	if loaded.Commands.Enter != cfg.Commands.Enter {
		t.Errorf("commands.enter mismatch: expected %q, got %q", cfg.Commands.Enter, loaded.Commands.Enter)
	}
	if len(loaded.Mounts) != len(cfg.Mounts) {
		t.Errorf("mounts count mismatch: expected %d, got %d", len(cfg.Mounts), len(loaded.Mounts))
	}
}
