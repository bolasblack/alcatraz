package util

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// sudoRun runs a command with sudo, connecting stdin/stdout/stderr.
func sudoRun(name string, args ...string) error {
	cmd := sudoCommand(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// sudoRunQuiet runs a command with sudo, suppressing output on success.
// On failure, returns the captured output along with the error.
func sudoRunQuiet(name string, args ...string) (string, error) {
	cmd := sudoCommand(name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// sudoRunScript writes script to a temp file and executes it with sudo.
func sudoRunScript(script string) error {
	tmpFile, err := os.CreateTemp("", "alcatraz-script-*.sh") //nolint:fslint
	if err != nil {
		return fmt.Errorf("failed to create temp script: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }() //nolint:fslint

	if _, err := tmpFile.WriteString(script); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write script: %w", err)
	}
	_ = tmpFile.Close()

	return sudoRun("sh", tmpFile.Name())
}

// sudoCommand creates an exec.Cmd for running a command with sudo.
func sudoCommand(name string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{name}, args...)
	return exec.Command("sudo", cmdArgs...)
}
