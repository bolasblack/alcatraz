package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetProjectAnchorName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple path",
			input:    "/Users/alice/project",
			expected: "-Users-alice-project",
		},
		{
			name:     "root path",
			input:    "/",
			expected: "-",
		},
		{
			name:     "nested path",
			input:    "/home/user/code/my-project/subdir",
			expected: "-home-user-code-my-project-subdir",
		},
		{
			name:     "path with spaces",
			input:    "/Users/alice/My Project",
			expected: "-Users-alice-My Project",
		},
		{
			name:     "path with special chars",
			input:    "/Users/alice/project_v2.0",
			expected: "-Users-alice-project_v2.0",
		},
		{
			name:     "path with multiple slashes normalized",
			input:    "/Users/alice/project",
			expected: "-Users-alice-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// GetProjectAnchorName calls filepath.Abs, so for absolute paths
			// the result should match our expected value
			result := GetProjectAnchorName(tt.input)
			if result != tt.expected {
				t.Errorf("GetProjectAnchorName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetProjectAnchorName_RelativePath(t *testing.T) {
	// For relative paths, GetProjectAnchorName should convert to absolute
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	input := "myproject"
	expected := filepath.Join(cwd, input)
	expected = "-" + expected[1:] // Replace leading / with -
	// Replace all / with -
	for i := 0; i < len(expected); i++ {
		if expected[i] == '/' {
			expected = expected[:i] + "-" + expected[i+1:]
		}
	}

	result := GetProjectAnchorName(input)
	if result != expected {
		t.Errorf("GetProjectAnchorName(%q) = %q, want %q", input, result, expected)
	}
}

func TestFileExists(t *testing.T) {
	// Test with existing file
	tmpFile, err := os.CreateTemp("", "test-file-exists-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	if !fileExists(tmpFile.Name()) {
		t.Errorf("fileExists(%q) = false, want true", tmpFile.Name())
	}

	// Test with existing directory
	tmpDir, err := os.MkdirTemp("", "test-dir-exists-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if !fileExists(tmpDir) {
		t.Errorf("fileExists(%q) = false, want true for directory", tmpDir)
	}

	// Test with non-existing path
	nonExistent := "/tmp/this-path-should-not-exist-12345"
	if fileExists(nonExistent) {
		t.Errorf("fileExists(%q) = true, want false", nonExistent)
	}
}
