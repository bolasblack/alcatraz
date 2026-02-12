package config

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestGenerateConfig(t *testing.T) {
	t.Run("writes valid TOML with expected fields", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		tc := TemplateConfig{
			Config: Config{
				Image: "myimage:latest",
				Mounts: []MountConfig{
					{Source: ".cache/data", Target: "/data"},
				},
				Commands: Commands{
					Up: CommandValue{Command: "echo hello"},
				},
				Envs: map[string]EnvValue{
					"FOO": {Value: "bar"},
				},
			},
			Includes: []string{"./.alca.*.toml"},
		}

		err := GenerateConfig(fs, "/project/.alca.toml", tc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, _ := afero.ReadFile(fs, "/project/.alca.toml")
		content := string(data)

		if !strings.Contains(content, SchemaComment) {
			t.Error("expected schema comment prefix")
		}
		if !strings.Contains(content, "image = 'myimage:latest'") {
			t.Errorf("expected image field in output:\n%s", content)
		}
		if !strings.Contains(content, "FOO = 'bar'") {
			t.Errorf("expected env FOO in output:\n%s", content)
		}
		if !strings.Contains(content, "up = 'echo hello'") {
			t.Errorf("expected up command in output:\n%s", content)
		}
	})

	t.Run("inserts comment before up command", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		tc := TemplateConfig{
			Config: Config{
				Image:    "img",
				Commands: Commands{Up: CommandValue{Command: "build"}},
			},
			UpComment: "prepare the env",
		}

		GenerateConfig(fs, "/p/.alca.toml", tc)
		data, _ := afero.ReadFile(fs, "/p/.alca.toml")
		content := string(data)

		if !strings.Contains(content, "# prepare the env\nup = ") {
			t.Errorf("expected comment immediately before up command:\n%s", content)
		}
	})

	t.Run("converts multiline commands to triple-quote format", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		tc := TemplateConfig{
			Config: Config{
				Image:    "img",
				Commands: Commands{Up: CommandValue{Command: "line1\nline2\nline3"}},
			},
		}

		GenerateConfig(fs, "/p/.alca.toml", tc)
		data, _ := afero.ReadFile(fs, "/p/.alca.toml")
		content := string(data)

		if !strings.Contains(content, `"""`) {
			t.Errorf("expected triple-quote format for multiline command:\n%s", content)
		}
		if strings.Contains(content, `\nline2`) {
			t.Errorf("should not contain escaped newlines:\n%s", content)
		}
	})

	t.Run("includes workdir_exclude in output", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		tc := TemplateConfig{
			Config: Config{
				Image:          "img",
				WorkdirExclude: []string{".env"},
			},
		}

		GenerateConfig(fs, "/p/.alca.toml", tc)
		data, _ := afero.ReadFile(fs, "/p/.alca.toml")

		if !strings.Contains(string(data), "workdir_exclude = ['.env']") {
			t.Errorf("expected workdir_exclude in output:\n%s", string(data))
		}
	})

	t.Run("includes enter command in output", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		tc := TemplateConfig{
			Config: Config{
				Image:    "img",
				Commands: Commands{Enter: CommandValue{Command: ". ~/.bashrc"}},
			},
		}

		GenerateConfig(fs, "/p/.alca.toml", tc)
		data, _ := afero.ReadFile(fs, "/p/.alca.toml")

		if !strings.Contains(string(data), "enter = '. ~/.bashrc'") {
			t.Errorf("expected enter command in output:\n%s", string(data))
		}
	})

	t.Run("simple env values use inline format", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		tc := TemplateConfig{
			Config: Config{
				Image: "img",
				Envs:  map[string]EnvValue{"MY_VAR": {Value: "1"}},
			},
		}

		GenerateConfig(fs, "/p/.alca.toml", tc)
		data, _ := afero.ReadFile(fs, "/p/.alca.toml")
		content := string(data)

		if strings.Contains(content, "[envs.MY_VAR]") {
			t.Errorf("simple env should not use sub-table format:\n%s", content)
		}
		if !strings.Contains(content, "MY_VAR = '1'") {
			t.Errorf("expected inline env format:\n%s", content)
		}
	})
}

func TestGenerateConfigGitignore(t *testing.T) {
	baseTc := TemplateConfig{
		Config:    Config{Image: "img"},
		Gitignore: []string{".alca.local.toml", ".alca.cache/"},
	}

	t.Run("creates gitignore when it does not exist", func(t *testing.T) {
		fs := afero.NewMemMapFs()

		if err := GenerateConfig(fs, "/project/.alca.toml", baseTc); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := afero.ReadFile(fs, "/project/.gitignore")
		if err != nil {
			t.Fatalf("expected .gitignore to be created: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, ".alca.local.toml") {
			t.Errorf("expected .alca.local.toml in .gitignore, got:\n%s", content)
		}
		if !strings.Contains(content, ".alca.cache/") {
			t.Errorf("expected .alca.cache/ in .gitignore, got:\n%s", content)
		}
	})

	t.Run("appends entries to existing gitignore", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/project/.gitignore", []byte("node_modules/\n"), 0644)

		if err := GenerateConfig(fs, "/project/.alca.toml", baseTc); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, _ := afero.ReadFile(fs, "/project/.gitignore")
		content := string(data)

		if !strings.Contains(content, "node_modules/") {
			t.Errorf("expected original content preserved, got:\n%s", content)
		}
		if !strings.Contains(content, ".alca.local.toml") {
			t.Errorf("expected .alca.local.toml in .gitignore, got:\n%s", content)
		}
		if !strings.Contains(content, ".alca.cache/") {
			t.Errorf("expected .alca.cache/ in .gitignore, got:\n%s", content)
		}
	})

	t.Run("does not duplicate existing entries", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/project/.gitignore", []byte(".alca.local.toml\n.alca.cache/\n"), 0644)

		if err := GenerateConfig(fs, "/project/.alca.toml", baseTc); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, _ := afero.ReadFile(fs, "/project/.gitignore")
		content := string(data)

		if strings.Count(content, ".alca.local.toml") != 1 {
			t.Errorf("expected exactly 1 occurrence of .alca.local.toml in:\n%s", content)
		}
		if strings.Count(content, ".alca.cache/") != 1 {
			t.Errorf("expected exactly 1 occurrence of .alca.cache/ in:\n%s", content)
		}
	})

	t.Run("adds newline separator when file does not end with newline", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		afero.WriteFile(fs, "/project/.gitignore", []byte("node_modules/"), 0644)

		if err := GenerateConfig(fs, "/project/.alca.toml", baseTc); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, _ := afero.ReadFile(fs, "/project/.gitignore")
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

		if lines[0] != "node_modules/" {
			t.Errorf("expected first line to be 'node_modules/', got %q", lines[0])
		}
		if lines[1] != ".alca.local.toml" {
			t.Errorf("expected second line to be '.alca.local.toml', got %q", lines[1])
		}
	})

	t.Run("skips gitignore when template has no entries", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		tc := TemplateConfig{Config: Config{Image: "img"}}

		if err := GenerateConfig(fs, "/project/.alca.toml", tc); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		exists, _ := afero.Exists(fs, "/project/.gitignore")
		if exists {
			t.Error("expected .gitignore to not be created when Gitignore is empty")
		}
	})
}

func TestGetTemplateConfigUnknownFallback(t *testing.T) {
	tc := GetTemplateConfig("unknown")
	if tc.Config.Image != "nixos/nix" {
		t.Errorf("expected unknown template to fall back to nix, got image %q", tc.Config.Image)
	}
}
