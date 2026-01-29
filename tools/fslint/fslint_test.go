package fslint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldScanPackage(t *testing.T) {
	scanDirs := []string{"internal/"}

	tests := []struct {
		pkgPath  string
		expected bool
	}{
		{"github.com/example/project/internal/foo", true},
		{"github.com/example/project/internal/foo/bar", true},
		{"github.com/example/project/cmd/foo", false},
		{"github.com/example/project/pkg/foo", false},
		{"internal/foo", true},
		{"internals/foo", false},
	}

	for _, tt := range tests {
		t.Run(tt.pkgPath, func(t *testing.T) {
			result := shouldScanPackage(tt.pkgPath, scanDirs)
			if result != tt.expected {
				t.Errorf("shouldScanPackage(%q, %v) = %v, want %v", tt.pkgPath, scanDirs, result, tt.expected)
			}
		})
	}
}

func TestIsAllowedPackage(t *testing.T) {
	allowedPackages := []string{"internal/transact", "internal/sudo"}

	tests := []struct {
		pkgPath  string
		expected bool
	}{
		{"github.com/example/project/internal/transact", true},
		{"github.com/example/project/internal/transact/sub", true},
		{"github.com/example/project/internal/foo", false},
		{"github.com/example/project/internal/transaction", false},
		{"github.com/example/project/internal/sudo", true},
		{"github.com/example/project/internal/sudo/helper", true},
		{"github.com/example/project/internal/sudoku", false},
		{"internal/transact", true},
		{"internal/transact/sub", true},
	}

	for _, tt := range tests {
		t.Run(tt.pkgPath, func(t *testing.T) {
			result := isAllowedPackage(tt.pkgPath, allowedPackages)
			if result != tt.expected {
				t.Errorf("isAllowedPackage(%q, %v) = %v, want %v", tt.pkgPath, allowedPackages, result, tt.expected)
			}
		})
	}
}

func TestMatchesPackagePath(t *testing.T) {
	tests := []struct {
		pkgPath  string
		pattern  string
		expected bool
	}{
		{"github.com/example/project/internal/transact", "internal/transact", true},
		{"github.com/example/project/internal/transact/sub", "internal/transact", true},
		{"github.com/example/project/internal/foo", "internal/transact", false},
		{"github.com/example/project/internal/transaction", "internal/transact", false},
		{"internal/transact", "internal/transact", true},
		{"internal/transact/sub", "internal/transact", true},
	}

	for _, tt := range tests {
		t.Run(tt.pkgPath+"_"+tt.pattern, func(t *testing.T) {
			result := matchesPackagePath(tt.pkgPath, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchesPackagePath(%q, %q) = %v, want %v", tt.pkgPath, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantErr     bool
		checkConfig func(*testing.T, *Config)
	}{
		{
			name: "valid config",
			content: `
scan_dirs = ["internal/"]
allowed_packages = ["internal/transact", "internal/sudo"]

[forbidden_calls]
"os" = ["Open", "Create"]
"io/ioutil" = ["ReadFile"]
`,
			wantErr: false,
			checkConfig: func(t *testing.T, cfg *Config) {
				if len(cfg.ScanDirs) != 1 {
					t.Errorf("expected 1 scan dir, got %d", len(cfg.ScanDirs))
				}
				if cfg.ScanDirs[0] != "internal/" {
					t.Errorf("expected scan dir to be 'internal/', got %q", cfg.ScanDirs[0])
				}
				if len(cfg.AllowedPackages) != 2 {
					t.Errorf("expected 2 allowed packages, got %d", len(cfg.AllowedPackages))
				}
				if cfg.AllowedPackages[0] != "internal/transact" {
					t.Errorf("expected first allowed package to be 'internal/transact', got %q", cfg.AllowedPackages[0])
				}
				if len(cfg.ForbiddenCalls) != 2 {
					t.Errorf("expected 2 forbidden call packages, got %d", len(cfg.ForbiddenCalls))
				}
				if len(cfg.ForbiddenCalls["os"]) != 2 {
					t.Errorf("expected 2 os functions, got %d", len(cfg.ForbiddenCalls["os"]))
				}
			},
		},
		{
			name: "empty config",
			content: `
scan_dirs = []
allowed_packages = []
[forbidden_calls]
`,
			wantErr: false,
			checkConfig: func(t *testing.T, cfg *Config) {
				if len(cfg.ScanDirs) != 0 {
					t.Errorf("expected 0 scan dirs, got %d", len(cfg.ScanDirs))
				}
				if len(cfg.AllowedPackages) != 0 {
					t.Errorf("expected 0 allowed packages, got %d", len(cfg.AllowedPackages))
				}
			},
		},
		{
			name:    "invalid toml",
			content: `this is not valid toml [`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "fslint.toml")
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write temp config: %v", err)
			}

			cfg, err := loadConfig(configPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.checkConfig != nil {
				tt.checkConfig(t, cfg)
			}
		})
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := loadConfig("/nonexistent/path/config.toml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	_, err := loadConfig("")
	if err == nil {
		t.Error("expected error for empty path, got nil")
	}
}
