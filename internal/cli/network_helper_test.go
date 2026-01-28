package cli

import (
	"os"
	"testing"
)

func TestFileExistsOS(t *testing.T) {
	// Test with existing file
	tmpFile, err := os.CreateTemp("", "test-file-exists-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	if !fileExistsOS(tmpFile.Name()) {
		t.Errorf("fileExistsOS(%q) = false, want true", tmpFile.Name())
	}

	// Test with existing directory
	tmpDir, err := os.MkdirTemp("", "test-dir-exists-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if !fileExistsOS(tmpDir) {
		t.Errorf("fileExistsOS(%q) = false, want true for directory", tmpDir)
	}

	// Test with non-existing path
	nonExistent := "/tmp/this-path-should-not-exist-12345"
	if fileExistsOS(nonExistent) {
		t.Errorf("fileExistsOS(%q) = true, want false", nonExistent)
	}
}
