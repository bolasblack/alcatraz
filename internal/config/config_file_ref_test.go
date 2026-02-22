package config

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

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

func TestConfigFileRef_Expand(t *testing.T) {
	_, memFs := newTestEnv(t)

	// Shared test files
	for _, f := range []string{
		"/test/a.toml",
		"/test/b.toml",
		"/test/c.txt",
		"/test/sub/x.toml",
		"/test/sub/y.toml",
	} {
		if err := afero.WriteFile(memFs, f, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	tests := []struct {
		name      string
		parentDir string
		path      string
		envVars   map[string]string // env var replacements for expandEnv
		want      []string          // expected matches
		wantErr   bool
	}{
		{
			name:      "literal path exists",
			parentDir: "/test",
			path:      "a.toml",
			want:      []string{"/test/a.toml"},
		},
		{
			name:      "literal absolute path ignores parentDir",
			parentDir: "/other",
			path:      "/test/a.toml",
			want:      []string{"/test/a.toml"},
		},
		{
			name:      "literal path not exists",
			parentDir: "/test",
			path:      "nonexistent.toml",
			wantErr:   true,
		},
		{
			name:      "glob matches sorted alphabetically",
			parentDir: "/test",
			path:      "*.toml",
			want:      []string{"/test/a.toml", "/test/b.toml"},
		},
		{
			name:      "glob no matches returns empty",
			parentDir: "/test",
			path:      "*.json",
			want:      []string{},
		},
		{
			name:      "malformed glob pattern returns error",
			parentDir: "/test",
			path:      "[unclosed",
			wantErr:   true,
		},
		{
			name:      "env var expansion to absolute path",
			parentDir: "/other",
			path:      "${MY_DIR}/a.toml",
			envVars:   map[string]string{"${MY_DIR}": "/test"},
			want:      []string{"/test/a.toml"},
		},
		{
			name:      "env var expansion with relative path and glob",
			parentDir: "/test",
			path:      "${SUB}/*.toml",
			envVars:   map[string]string{"${SUB}": "sub"},
			want:      []string{"/test/sub/x.toml", "/test/sub/y.toml"},
		},
		{
			name:      "env var expanding to glob characters triggers glob",
			parentDir: "/test",
			path:      "${PATTERN}",
			envVars:   map[string]string{"${PATTERN}": "*.toml"},
			want:      []string{"/test/a.toml", "/test/b.toml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandEnv := func(s string) string {
				for k, v := range tt.envVars {
					s = strings.ReplaceAll(s, k, v)
				}
				return s
			}

			ref := NewConfigFileRef(tt.parentDir, tt.path)
			got, err := ref.Expand(expandEnv, memFs)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
