package util

import (
	"context"
	"os/exec"
)

// CommandRunner executes external commands.
type CommandRunner interface {
	// Run executes a command and returns combined stdout/stderr.
	Run(name string, args ...string) (output []byte, err error)

	// RunQuiet executes a command, returns output only on error.
	RunQuiet(name string, args ...string) (output string, err error)

	// SudoRun runs a command with sudo, connecting stdin/stdout/stderr.
	SudoRun(name string, args ...string) error

	// SudoRunQuiet runs a command with sudo, returns output only on error.
	SudoRunQuiet(name string, args ...string) (output string, err error)

	// SudoRunScript writes script to a temp file and executes it with sudo.
	SudoRunScript(script string) error
}

// ContextCommandRunner implements CommandRunner with context support.
type ContextCommandRunner struct {
	ctx context.Context
}

// NewCommandRunner creates a new ContextCommandRunner with context.Background().
func NewCommandRunner() *ContextCommandRunner {
	return &ContextCommandRunner{ctx: context.Background()}
}

// WithContext returns a new ContextCommandRunner with the given context.
func (r *ContextCommandRunner) WithContext(ctx context.Context) *ContextCommandRunner {
	return &ContextCommandRunner{ctx: ctx}
}

func (r *ContextCommandRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(r.ctx, name, args...) //nolint:fslint // CommandRunner is the abstraction layer
	return cmd.CombinedOutput()
}

func (r *ContextCommandRunner) RunQuiet(name string, args ...string) (string, error) {
	cmd := exec.CommandContext(r.ctx, name, args...) //nolint:fslint // CommandRunner is the abstraction layer
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return "", nil
}

func (r *ContextCommandRunner) SudoRun(name string, args ...string) error {
	return sudoRunContext(r.ctx, name, args...)
}

func (r *ContextCommandRunner) SudoRunQuiet(name string, args ...string) (string, error) {
	return sudoRunQuietContext(r.ctx, name, args...)
}

func (r *ContextCommandRunner) SudoRunScript(script string) error {
	return sudoRunScriptContext(r.ctx, script)
}
