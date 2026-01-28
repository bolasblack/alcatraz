package transact

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"golang.org/x/sys/unix"
)

// OpType represents the type of file operation.
type OpType int

const (
	// OpCreate indicates a new file creation.
	OpCreate OpType = iota
	// OpUpdate indicates updating an existing file's content.
	OpUpdate
	// OpChmod indicates a permission change only.
	OpChmod
	// OpDelete indicates file deletion.
	OpDelete
)

// String returns a human-readable string for the operation type.
func (o OpType) String() string {
	switch o {
	case OpCreate:
		return "create"
	case OpUpdate:
		return "update"
	case OpChmod:
		return "chmod"
	case OpDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// FileOp represents a single file operation to be committed.
// See AGD-015 for the struct field exhaustiveness pattern.
type FileOp struct {
	Path     string
	Op       OpType
	Content  []byte
	Mode     os.FileMode
	NeedSudo bool
}

// enforceFileOpFieldCompleteness ensures all FileOp fields are handled.
// See AGD-015 for pattern details.
func enforceFileOpFieldCompleteness(op *FileOp) {
	type fields struct {
		Path     string
		Op       OpType
		Content  []byte
		Mode     os.FileMode
		NeedSudo bool
	}
	_ = fields(*op)
}

// ComputeDiff compares staged vs actual filesystem and returns operations needed.
func ComputeDiff(staged, actual afero.Fs, paths, deletedPaths []string) ([]FileOp, error) {
	var ops []FileOp

	// Handle deletions first
	for _, path := range deletedPaths {
		_, err := actual.Stat(path)
		if err == nil {
			// File exists in actual, needs deletion
			op := FileOp{
				Path:     path,
				Op:       OpDelete,
				NeedSudo: needsSudo(path),
			}
			enforceFileOpFieldCompleteness(&op)
			ops = append(ops, op)
		}
		// If file doesn't exist in actual, no-op
	}

	// Handle creates/updates/chmod
	for _, path := range paths {
		stagedInfo, stagedErr := staged.Stat(path)
		actualInfo, actualErr := actual.Stat(path)

		switch {
		case stagedErr != nil && actualErr == nil:
			// File deleted in staged (should be in deletedPaths, handled above)
			continue

		case stagedErr == nil && actualErr != nil:
			// New file - doesn't exist in actual
			content, err := afero.ReadFile(staged, path)
			if err != nil {
				return nil, err
			}
			op := FileOp{
				Path:     path,
				Op:       OpCreate,
				Content:  content,
				Mode:     stagedInfo.Mode().Perm(),
				NeedSudo: needsSudo(path),
			}
			enforceFileOpFieldCompleteness(&op)
			ops = append(ops, op)

		case stagedErr == nil && actualErr == nil:
			// Both exist - check content and permissions
			stagedContent, err := afero.ReadFile(staged, path)
			if err != nil {
				return nil, err
			}
			actualContent, err := afero.ReadFile(actual, path)
			if err != nil {
				return nil, err
			}

			contentChanged := !bytes.Equal(stagedContent, actualContent)
			modeChanged := stagedInfo.Mode().Perm() != actualInfo.Mode().Perm()

			if contentChanged {
				op := FileOp{
					Path:     path,
					Op:       OpUpdate,
					Content:  stagedContent,
					Mode:     stagedInfo.Mode().Perm(),
					NeedSudo: needsSudo(path),
				}
				enforceFileOpFieldCompleteness(&op)
				ops = append(ops, op)
			} else if modeChanged {
				op := FileOp{
					Path:     path,
					Op:       OpChmod,
					Mode:     stagedInfo.Mode().Perm(),
					NeedSudo: needsSudo(path),
				}
				enforceFileOpFieldCompleteness(&op)
				ops = append(ops, op)
			}
			// If neither changed, no-op
		}
	}

	return ops, nil
}

// needsSudo determines if a path requires sudo for modification.
// Checks actual write permission using unix.Access().
func needsSudo(path string) bool {
	// Check if we can write to the file
	if err := unix.Access(path, unix.W_OK); err == nil {
		return false // Can write without sudo
	}
	// File doesn't exist or no permission, check parent directory
	dir := filepath.Dir(path)
	if err := unix.Access(dir, unix.W_OK); err == nil {
		return false // Can write to parent without sudo
	}
	return true // Need sudo
}
