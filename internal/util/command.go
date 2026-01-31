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
	Run(name string, args ...string) (output []byte, err error)

	// RunQuiet executes a command without streaming, returning combined stdout/stderr.
	RunQuiet(name string, args ...string) (output []byte, err error)

	// SudoRun runs a command with sudo, connecting stdin/stdout/stderr.
	SudoRun(name string, args ...string) error

	// SudoRunQuiet runs a command with sudo without streaming, returning combined stdout/stderr.
	SudoRunQuiet(name string, args ...string) (output []byte, err error)

	// SudoRunScriptQuiet writes script to a temp file and executes it with sudo.
	SudoRunScriptQuiet(script string) error
}

// ContextCommandRunner implements CommandRunner with context support.
type ContextCommandRunner struct {
	ctx    context.Context
	stdout io.Writer
	stderr io.Writer
}

// NewCommandRunner creates a new ContextCommandRunner with context.Background().
func NewCommandRunner() *ContextCommandRunner {
	return &ContextCommandRunner{
		ctx:    context.Background(),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

// WithContext returns a new ContextCommandRunner with the given context.
func (r *ContextCommandRunner) WithContext(ctx context.Context) *ContextCommandRunner {
	return &ContextCommandRunner{ctx: ctx, stdout: r.stdout, stderr: r.stderr}
}

func (r *ContextCommandRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(r.ctx, name, args...) //nolint:fslint // CommandRunner is the abstraction layer
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(r.stdout, &buf)
	cmd.Stderr = io.MultiWriter(r.stderr, &buf)
	err := cmd.Run()
	return buf.Bytes(), err
}

func (r *ContextCommandRunner) RunQuiet(name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(r.ctx, name, args...) //nolint:fslint // CommandRunner is the abstraction layer
	return cmd.CombinedOutput()
}

func (r *ContextCommandRunner) SudoRun(name string, args ...string) error {
	return sudoRunContext(r.ctx, name, args...)
}

func (r *ContextCommandRunner) SudoRunQuiet(name string, args ...string) ([]byte, error) {
	return sudoRunQuietContext(r.ctx, name, args...)
}

func (r *ContextCommandRunner) SudoRunScriptQuiet(script string) error {
	return sudoRunScriptContext(r.ctx, script)
}
