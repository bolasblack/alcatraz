package util

import (
	"github.com/spf13/afero"
)

// Env contains environment dependencies that can be mocked for testing.
type Env struct {
	// Fs is the filesystem to use for file operations.
	Fs afero.Fs
	// Cmd is the command runner for executing external commands.
	Cmd CommandRunner
}

// NewEnv creates an Env with the given filesystem.
// For production use, pass transact.New() to enable batched file operations.
func NewEnv(fs afero.Fs) *Env {
	return &Env{Fs: fs, Cmd: NewCommandRunner()}
}

// NewReadonlyOsEnv creates an Env with a read-only OS filesystem.
// Use this for commands that only read files (like status, list, run).
// Write operations will fail with an error.
func NewReadonlyOsEnv() *Env {
	return &Env{Fs: afero.NewReadOnlyFs(afero.NewOsFs()), Cmd: NewCommandRunner()}
}

// NewTestEnv creates an Env with in-memory filesystem and mock command runner (for testing).
func NewTestEnv() *Env {
	return &Env{
		Fs:  afero.NewMemMapFs(),
		Cmd: NewMockCommandRunner(),
	}
}

// WithCommandRunner returns a copy with the given command runner.
func (e *Env) WithCommandRunner(cmd CommandRunner) *Env {
	return &Env{Fs: e.Fs, Cmd: cmd}
}
