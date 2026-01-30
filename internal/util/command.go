package util

import "os/exec"

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

// DefaultCommandRunner uses os/exec.
type DefaultCommandRunner struct{}

func (DefaultCommandRunner) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

func (DefaultCommandRunner) RunQuiet(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return "", nil
}

func (DefaultCommandRunner) SudoRun(name string, args ...string) error {
	return sudoRun(name, args...)
}

func (DefaultCommandRunner) SudoRunQuiet(name string, args ...string) (string, error) {
	return sudoRunQuiet(name, args...)
}

func (DefaultCommandRunner) SudoRunScript(script string) error {
	return sudoRunScript(script)
}
