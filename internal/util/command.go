package util

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
)

// CommandRunner executes external commands.
type CommandRunner interface {
	// Run executes a command and returns combined stdout/stderr.
	Run(ctx context.Context, name string, args ...string) (output []byte, err error)

	// RunQuiet executes a command without streaming, returning combined stdout/stderr.
	RunQuiet(ctx context.Context, name string, args ...string) (output []byte, err error)

	// SudoRun runs a command with sudo, connecting stdin/stdout/stderr.
	SudoRun(ctx context.Context, name string, args ...string) error

	// SudoRunQuiet runs a command with sudo without streaming, returning combined stdout/stderr.
	SudoRunQuiet(ctx context.Context, name string, args ...string) (output []byte, err error)

	// SudoRunScriptQuiet writes script to a temp file and executes it with sudo.
	SudoRunScriptQuiet(ctx context.Context, script string) error
}

var _ CommandRunner = (*DefaultCommandRunner)(nil)

// DefaultCommandRunner implements CommandRunner using os/exec.
type DefaultCommandRunner struct {
	stdout io.Writer
	stderr io.Writer
}

// NewCommandRunner creates a new DefaultCommandRunner.
func NewCommandRunner() *DefaultCommandRunner {
	return &DefaultCommandRunner{
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

func (r *DefaultCommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:fslint // CommandRunner is the abstraction layer
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(r.stdout, &buf)
	cmd.Stderr = io.MultiWriter(r.stderr, &buf)
	err := cmd.Run()
	return buf.Bytes(), err
}

func (r *DefaultCommandRunner) RunQuiet(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:fslint // CommandRunner is the abstraction layer
	return cmd.CombinedOutput()
}

func (r *DefaultCommandRunner) SudoRun(ctx context.Context, name string, args ...string) error {
	return sudoRunContext(ctx, name, args...)
}

func (r *DefaultCommandRunner) SudoRunQuiet(ctx context.Context, name string, args ...string) ([]byte, error) {
	return sudoRunQuietContext(ctx, name, args...)
}

func (r *DefaultCommandRunner) SudoRunScriptQuiet(ctx context.Context, script string) error {
	return sudoRunScriptContext(ctx, script)
}
