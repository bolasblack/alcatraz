// Package sudo provides helpers for running privileged operations via sudo.
package sudo

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run runs a command with sudo.
func Run(name string, args ...string) error {
	cmd := sudoCommand(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunQuiet runs a command with sudo, suppressing output on success.
// On failure, returns the captured output along with the error.
func RunQuiet(name string, args ...string) (output string, err error) {
	cmd := sudoCommand(name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// RunScript executes a shell script with sudo.
// The script is written to a temp file and executed via sudo sh.
func RunScript(script string) error {
	tmpFile, err := os.CreateTemp("", "alcatraz-script-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temp script: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write script: %w", err)
	}
	tmpFile.Close()

	return Run("sh", tmpFile.Name())
}

// sudoCommand creates an exec.Cmd for running a command with sudo.
func sudoCommand(name string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{name}, args...)
	return exec.Command("sudo", cmdArgs...)
}
