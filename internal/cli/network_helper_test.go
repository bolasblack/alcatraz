package cli

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

func TestFileExists(t *testing.T) {
	env := util.NewTestEnv()

	// Test with existing file
	if err := afero.WriteFile(env.Fs, "/test-file", []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	if !fileExists(env, "/test-file") {
		t.Errorf("fileExists(/test-file) = false, want true")
	}

	// Test with existing directory
	if err := env.Fs.MkdirAll("/test-dir", 0755); err != nil {
		t.Fatalf("failed to create test dir: %v", err)
	}

	if !fileExists(env, "/test-dir") {
		t.Errorf("fileExists(/test-dir) = false, want true for directory")
	}

	// Test with non-existing path
	if fileExists(env, "/non-existent") {
		t.Errorf("fileExists(/non-existent) = true, want false")
	}
}
