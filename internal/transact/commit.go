package transact

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/spf13/afero"
)

// SudoRunner is the interface for executing sudo commands.
// Used by commit callbacks that need to execute privileged operations.
type SudoRunner interface {
	Run(name string, args ...string) error
}

// ExecuteOp executes a single file operation on the given filesystem.
// Useful for implementing commit callbacks.
func ExecuteOp(fs afero.Fs, op FileOp) error {
	switch op.Op {
	case OpCreate, OpUpdate:
		// Ensure parent directory exists
		dir := parentDir(op.Path)
		if err := fs.MkdirAll(dir, 0755); err != nil {
			return err
		}
		if err := afero.WriteFile(fs, op.Path, op.Content, op.Mode); err != nil {
			return err
		}
		return nil

	case OpChmod:
		return fs.Chmod(op.Path, op.Mode)

	case OpDelete:
		return fs.Remove(op.Path)

	default:
		return fmt.Errorf("unknown operation type: %d", op.Op)
	}
}

// GenerateBatchScript creates a shell script for batched sudo operations.
// Uses base64 encoding for file content to avoid shell escaping issues.
// Useful for implementing commit callbacks that need sudo.
func GenerateBatchScript(ops []FileOp) string {
	var script strings.Builder
	script.WriteString("set -e\n") // Exit on first error

	for _, op := range ops {
		switch op.Op {
		case OpCreate, OpUpdate:
			// Ensure parent directory exists
			dir := parentDir(op.Path)
			script.WriteString(fmt.Sprintf("mkdir -p %q\n", dir))

			// Use base64 for safe content transfer
			encoded := base64.StdEncoding.EncodeToString(op.Content)
			script.WriteString(fmt.Sprintf("echo %q | base64 -d > %q\n", encoded, op.Path))
			script.WriteString(fmt.Sprintf("chmod %o %q\n", op.Mode.Perm(), op.Path))

		case OpChmod:
			script.WriteString(fmt.Sprintf("chmod %o %q\n", op.Mode.Perm(), op.Path))

		case OpDelete:
			script.WriteString(fmt.Sprintf("rm -f %q\n", op.Path))
		}
	}

	return script.String()
}
