// Package sudo provides helpers for running privileged operations via sudo.
package sudo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run runs a command with sudo.
func Run(name string, args ...string) error {
	cmdArgs := append([]string{name}, args...)
	cmd := exec.Command("sudo", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunQuiet runs a command with sudo, suppressing output on success.
// On failure, returns the captured output along with the error.
func RunQuiet(name string, args ...string) (output string, err error) {
	cmdArgs := append([]string{name}, args...)
	cmd := exec.Command("sudo", cmdArgs...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// EnsurePath checks if directory exists, runs sudo mkdir -p if needed.
func EnsurePath(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := Run("mkdir", "-p", path); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return nil
}

// EnsureChmod checks permissions and runs sudo chmod if needed.
// If recursive=true, checks all files and uses chmod -R if any differ.
func EnsureChmod(path string, mode os.FileMode, recursive bool) error {
	if recursive {
		needsChange := false
		err := filepath.Walk(path, func(p string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				needsChange = true // can't check, assume change needed
				return nil         // continue walking
			}
			if info.Mode().Perm() != mode {
				needsChange = true
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to walk directory: %w", err)
		}
		if needsChange {
			if err := Run("chmod", "-R", fmt.Sprintf("%o", mode), path); err != nil {
				return fmt.Errorf("failed to set permissions: %w", err)
			}
		}
	} else {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			if err := Run("chmod", fmt.Sprintf("%o", mode), path); err != nil {
				return fmt.Errorf("failed to set permissions: %w", err)
			}
		}
	}
	return nil
}

// EnsureFileContent checks content and runs sudo cp if needed.
// Returns (false, nil) if content unchanged, (true, nil) after successful write.
func EnsureFileContent(path string, content string) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil && string(existing) == content {
		return false, nil
	}

	tmpFile, err := os.CreateTemp("", "alcatraz-*.txt")
	if err != nil {
		return false, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return false, fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	if err := Run("cp", tmpFile.Name(), path); err != nil {
		return false, fmt.Errorf("failed to copy file: %w", err)
	}
	return true, nil
}
