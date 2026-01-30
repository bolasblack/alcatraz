package util

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// sudoRunContext runs a command with sudo and context support.
func sudoRunContext(ctx context.Context, name string, args ...string) error {
	cmd := sudoCommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// sudoRunQuietContext runs a command with sudo and context, suppressing output on success.
func sudoRunQuietContext(ctx context.Context, name string, args ...string) (string, error) {
	cmd := sudoCommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// sudoRunScriptContext writes script to a temp file and executes it with sudo.
func sudoRunScriptContext(ctx context.Context, script string) error {
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

	return sudoRunContext(ctx, "sh", tmpFile.Name())
}

// sudoCommandContext creates an exec.Cmd for running a command with sudo and context.
func sudoCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmdArgs := append([]string{name}, args...)
	return exec.CommandContext(ctx, "sudo", cmdArgs...) //nolint:fslint // CommandRunner is the abstraction layer
}
