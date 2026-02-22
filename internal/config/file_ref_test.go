package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestFileRef_Expand(t *testing.T) {
	tests := []struct {
		name               string
		fromConfigFilePath string
		path               string
		envVars            map[string]string
		want               string
	}{
		{
			name:               "relative path resolved against config file directory",
			fromConfigFilePath: "/base/dir/config.toml",
			path:               "sub/file.toml",
			want:               "/base/dir/sub/file.toml",
		},
		{
			name:               "absolute path ignores config file directory",
			fromConfigFilePath: "/base/dir/config.toml",
			path:               "/absolute/file.toml",
			want:               "/absolute/file.toml",
		},
		{
			name:               "env var expansion with relative path",
			fromConfigFilePath: "/base/dir/config.toml",
			path:               "${SUB}/file.toml",
			envVars:            map[string]string{"${SUB}": "expanded"},
			want:               "/base/dir/expanded/file.toml",
		},
		{
			name:               "env var expansion producing absolute path",
			fromConfigFilePath: "/base/dir/config.toml",
			path:               "${DIR}/file.toml",
			envVars:            map[string]string{"${DIR}": "/absolute"},
			want:               "/absolute/file.toml",
		},
		{
			name:               "identity getenv returns path as-is",
			fromConfigFilePath: "/base/dir/config.toml",
			path:               "plain.toml",
			want:               "/base/dir/plain.toml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expandEnv := func(s string) (string, error) {
				for k, v := range tt.envVars {
					s = strings.ReplaceAll(s, k, v)
				}
				return s, nil
			}

			ref := NewFileRef(tt.fromConfigFilePath, tt.path)
			got, err := ref.Expand(expandEnv)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("Expand() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFileRef_Expand_ErrorPropagation(t *testing.T) {
	expandEnv := func(s string) (string, error) {
		return "", fmt.Errorf("undefined environment variable: $MISSING")
	}

	ref := NewFileRef("/base/dir/config.toml", "${MISSING}/file.toml")
	_, err := ref.Expand(expandEnv)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "undefined environment variable: $MISSING") {
		t.Errorf("expected error containing 'undefined environment variable: $MISSING', got: %v", err)
	}
}
