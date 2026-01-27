package cli

import (
	"os"
	"testing"

	"github.com/bolasblack/alcatraz/internal/network"
)

func TestFileExists(t *testing.T) {
	// Test with existing file
	tmpFile, err := os.CreateTemp("", "test-file-exists-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	if !network.FileExists(tmpFile.Name()) {
		t.Errorf("network.FileExists(%q) = false, want true", tmpFile.Name())
	}

	// Test with existing directory
	tmpDir, err := os.MkdirTemp("", "test-dir-exists-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if !network.FileExists(tmpDir) {
		t.Errorf("network.FileExists(%q) = false, want true for directory", tmpDir)
	}

	// Test with non-existing path
	nonExistent := "/tmp/this-path-should-not-exist-12345"
	if network.FileExists(nonExistent) {
		t.Errorf("network.FileExists(%q) = true, want false", nonExistent)
	}
}
