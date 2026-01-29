package util

import "github.com/spf13/afero"

// Env contains environment dependencies that can be mocked for testing.
type Env struct {
	// Fs is the filesystem to use for file operations.
	Fs afero.Fs
}

// NewEnv creates an Env with the given filesystem.
// For production use, pass transact.New() to enable batched file operations.
func NewEnv(fs afero.Fs) *Env {
	return &Env{Fs: fs}
}

// NewReadonlyOsEnv creates an Env with a read-only OS filesystem.
// Use this for commands that only read files (like status, list, run).
// Write operations will fail with an error.
func NewReadonlyOsEnv() *Env {
	return &Env{Fs: afero.NewReadOnlyFs(afero.NewOsFs())}
}

// NewTestEnv creates an Env with in-memory filesystem (for testing).
func NewTestEnv() *Env {
	return &Env{Fs: afero.NewMemMapFs()}
}
