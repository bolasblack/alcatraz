package transact

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/spf13/afero"
)

// OpGroup represents a group of operations with the same sudo requirement.
type OpGroup struct {
	NeedSudo bool
	Ops      []FileOp
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

// ExecuteOps executes multiple file operations on the given filesystem.
func ExecuteOps(fs afero.Fs, ops []FileOp) error {
	for _, op := range ops {
		if err := ExecuteOp(fs, op); err != nil {
			return fmt.Errorf("failed to execute %s on %s: %w", op.Op, op.Path, err)
		}
	}
	return nil
}

// GroupOpsBySudo groups operations by NeedSudo field while preserving order.
// Example: [false, false, true, true, false] -> [[false, false], [true, true], [false]]
func GroupOpsBySudo(ops []FileOp) []OpGroup {
	if len(ops) == 0 {
		return nil
	}

	var groups []OpGroup
	currentGroup := OpGroup{
		NeedSudo: ops[0].NeedSudo,
		Ops:      []FileOp{ops[0]},
	}

	for i := 1; i < len(ops); i++ {
		if ops[i].NeedSudo == currentGroup.NeedSudo {
			currentGroup.Ops = append(currentGroup.Ops, ops[i])
		} else {
			groups = append(groups, currentGroup)
			currentGroup = OpGroup{
				NeedSudo: ops[i].NeedSudo,
				Ops:      []FileOp{ops[i]},
			}
		}
	}
	groups = append(groups, currentGroup)

	return groups
}

// SudoExecutor is a function type for executing sudo operations.
// It receives the batch script and should execute it with sudo.
type SudoExecutor func(script string) error

// ExecuteGroupedOps executes grouped operations, using regular execution for
// non-sudo ops and the provided sudoExecutor for sudo ops.
func ExecuteGroupedOps(fs afero.Fs, groups []OpGroup, sudoExecutor SudoExecutor) error {
	for _, group := range groups {
		if group.NeedSudo {
			script := GenerateBatchScript(group.Ops)
			if err := sudoExecutor(script); err != nil {
				return fmt.Errorf("sudo execution failed: %w", err)
			}
		} else {
			if err := ExecuteOps(fs, group.Ops); err != nil {
				return err
			}
		}
	}
	return nil
}
