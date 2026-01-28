// Package transact provides transactional file system operations.
// It allows staging file changes in memory, computing diffs against the real filesystem,
// and committing changes with callback-based operations.
// See AGD-023 for design decisions.
package transact

import (
	"os"
	"slices"
	"sync"
	"time"

	"github.com/spf13/afero"
)

// =============================================================================
// Commit Types
// =============================================================================

// CommitContext contains all information needed for commit callback.
type CommitContext struct {
	// BaseFs is the actual filesystem to write to.
	BaseFs afero.Fs
	// Ops is the list of operations to perform.
	Ops []FileOp
}

// CommitOpsResult is the result returned by the commit callback.
type CommitOpsResult struct {
	// Future fields: FilesWritten, BytesWritten, etc.
}

// CommitResult is the result returned by Commit.
type CommitResult struct{}

// CommitFunc is the callback type for Commit.
type CommitFunc func(ctx CommitContext) (*CommitOpsResult, error)

// =============================================================================
// TransactFs
// =============================================================================

// Compile-time check: TransactFs implements afero.Fs
var _ afero.Fs = (*TransactFs)(nil)

// TransactFs wraps a staged (in-memory) filesystem with the actual filesystem,
// enabling transactional file operations with diff-based commits.
//
// Semantics:
//   - WriteFile/Chmod/Remove stage changes in memory
//   - ReadFile reads from staged first, then actual (CopyOnWrite)
//   - Open/OpenFile return wrappers that read from CopyOnWrite overlay
//   - Diff compares staged vs actual
//   - Commit applies staged changes via callback, then resets staged
type TransactFs struct {
	// staged is the in-memory filesystem for staging changes
	staged afero.Fs
	// actual is the real filesystem (typically OsFs, MemMapFs for tests)
	actual afero.Fs
	// paths tracks all paths that have been written/modified
	paths []string
	// deletedPaths tracks paths marked for deletion
	deletedPaths []string
	// openHandles tracks all open file handles for snapshot on delete
	openHandles map[*TransactFsFile]struct{}
	// mu protects concurrent access
	mu sync.RWMutex
}

// Option configures a TransactFs.
type Option func(*TransactFs)

// WithActualFs sets the actual filesystem (default: OsFs).
// Useful for testing with a mock filesystem.
func WithActualFs(fs afero.Fs) Option {
	return func(t *TransactFs) {
		t.actual = fs
	}
}

// New creates a new TransactFs with default OsFs for the actual filesystem.
func New(opts ...Option) *TransactFs {
	t := &TransactFs{
		staged:      afero.NewMemMapFs(),
		actual:      afero.NewOsFs(),
		openHandles: make(map[*TransactFsFile]struct{}),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// -----------------------------------------------------------------------------
// TransactFs: afero.Fs interface methods
// -----------------------------------------------------------------------------

// MkdirAll creates a directory and all parent directories in the staged filesystem.
func (t *TransactFs) MkdirAll(path string, perm os.FileMode) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.staged.MkdirAll(path, perm)
}

// Open opens a file for reading from the CopyOnWrite overlay.
func (t *TransactFs) Open(name string) (afero.File, error) {
	return t.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile opens a file with specified flags.
// Read operations use CopyOnWrite overlay.
// Write operations go to staged filesystem.
func (t *TransactFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if marked for deletion (unless creating)
	if slices.Contains(t.deletedPaths, name) && flag&os.O_CREATE == 0 {
		return nil, os.ErrNotExist
	}

	// For write modes, ensure the file exists in staged
	isWrite := flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC) != 0
	if isWrite {
		// Resurrect if was deleted
		t.deletedPaths = slices.DeleteFunc(t.deletedPaths, func(p string) bool {
			return p == name
		})

		// Ensure parent dir in staged
		if err := t.staged.MkdirAll(parentDir(name), 0755); err != nil {
			return nil, err
		}

		// If file doesn't exist in staged but exists in actual, copy it
		if _, err := t.staged.Stat(name); os.IsNotExist(err) {
			if content, err := afero.ReadFile(t.actual, name); err == nil {
				info, _ := t.actual.Stat(name)
				afero.WriteFile(t.staged, name, content, info.Mode().Perm())
			} else if flag&os.O_CREATE == 0 {
				// File doesn't exist anywhere and not creating
				return nil, err
			}
		}

		// Open from staged for write
		f, err := t.staged.OpenFile(name, flag, perm)
		if err != nil {
			return nil, err
		}
		t.trackPath(name)

		wrapper := &TransactFsFile{
			tfs:     t,
			path:    name,
			inner:   f,
			isWrite: true,
		}
		t.openHandles[wrapper] = struct{}{}
		return wrapper, nil
	}

	// Read-only mode: verify file exists
	existsInStaged := false
	existsInActual := false
	if _, err := t.staged.Stat(name); err == nil {
		existsInStaged = true
	}
	if _, err := t.actual.Stat(name); err == nil {
		existsInActual = true
	}

	if !existsInStaged && !existsInActual {
		return nil, os.ErrNotExist
	}

	wrapper := &TransactFsFile{
		tfs:     t,
		path:    name,
		isWrite: false,
	}
	t.openHandles[wrapper] = struct{}{}
	return wrapper, nil
}

// Chmod stages a permission change.
func (t *TransactFs) Chmod(path string, mode os.FileMode) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If file not in staged, copy from actual
	exists, err := afero.Exists(t.staged, path)
	if err != nil {
		return err
	}
	if !exists {
		content, err := afero.ReadFile(t.actual, path)
		if err != nil {
			return err
		}
		info, err := t.actual.Stat(path)
		if err != nil {
			return err
		}
		// Ensure parent dir
		if err := t.staged.MkdirAll(parentDir(path), 0755); err != nil {
			return err
		}
		if err := afero.WriteFile(t.staged, path, content, info.Mode().Perm()); err != nil {
			return err
		}
	}

	if err := t.staged.Chmod(path, mode); err != nil {
		return err
	}
	t.trackPath(path)
	return nil
}

// Remove stages a file deletion.
func (t *TransactFs) Remove(path string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Check if already deleted
	if slices.Contains(t.deletedPaths, path) {
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
	}

	// Check if file exists in staged or actual
	_, stagedErr := t.staged.Stat(path)
	_, actualErr := t.actual.Stat(path)
	if stagedErr != nil && actualErr != nil {
		return &os.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
	}

	// Snapshot content for any open handles on this path
	for handle := range t.openHandles {
		if handle.path == path && !handle.deleted {
			// Read current content from cow overlay
			content, err := t.readFileLocked(path)
			if err == nil {
				handle.snapshot = content
			}
			handle.deleted = true
		}
	}

	// Remove from staged if exists
	_ = t.staged.Remove(path)

	// Remove from tracked paths
	t.paths = slices.DeleteFunc(t.paths, func(p string) bool {
		return p == path
	})

	// Mark for deletion
	if !slices.Contains(t.deletedPaths, path) {
		t.deletedPaths = append(t.deletedPaths, path)
	}
	return nil
}

// RemoveAll removes a directory path and all children.
func (t *TransactFs) RemoveAll(path string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Remove from staged
	_ = t.staged.RemoveAll(path)

	// Mark for deletion (the actual removal happens at commit)
	if !slices.Contains(t.deletedPaths, path) {
		t.deletedPaths = append(t.deletedPaths, path)
	}
	return nil
}

// Create creates a file in the staged filesystem.
func (t *TransactFs) Create(name string) (afero.File, error) {
	return t.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

// Mkdir creates a directory in the staged filesystem.
func (t *TransactFs) Mkdir(name string, perm os.FileMode) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.staged.Mkdir(name, perm)
}

// Rename renames a file in the staged filesystem.
func (t *TransactFs) Rename(oldname, newname string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Read content from cow overlay
	content, err := t.readFileLocked(oldname)
	if err != nil {
		return err
	}

	// Get file info for mode
	var mode os.FileMode = 0644
	if info, err := t.staged.Stat(oldname); err == nil {
		mode = info.Mode().Perm()
	} else if info, err := t.actual.Stat(oldname); err == nil {
		mode = info.Mode().Perm()
	}

	// Ensure parent dir for new name
	if err := t.staged.MkdirAll(parentDir(newname), 0755); err != nil {
		return err
	}

	// Write to new location
	if err := afero.WriteFile(t.staged, newname, content, mode); err != nil {
		return err
	}
	t.trackPath(newname)

	// Mark old for deletion
	_ = t.staged.Remove(oldname)
	t.paths = slices.DeleteFunc(t.paths, func(p string) bool {
		return p == oldname
	})
	if !slices.Contains(t.deletedPaths, oldname) {
		t.deletedPaths = append(t.deletedPaths, oldname)
	}

	return nil
}

// Stat returns file info from the CopyOnWrite overlay (staged first, then actual).
func (t *TransactFs) Stat(name string) (os.FileInfo, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Check if marked for deletion
	if slices.Contains(t.deletedPaths, name) {
		return nil, os.ErrNotExist
	}

	// Try staged first
	if info, err := t.staged.Stat(name); err == nil {
		return info, nil
	}

	// Fallback to actual
	return t.actual.Stat(name)
}

// Name returns the name of this filesystem.
func (t *TransactFs) Name() string {
	return "TransactFs"
}

// Chown stages an ownership change.
func (t *TransactFs) Chown(name string, uid, gid int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If file not in staged, copy from actual
	exists, err := afero.Exists(t.staged, name)
	if err != nil {
		return err
	}
	if !exists {
		content, err := afero.ReadFile(t.actual, name)
		if err != nil {
			return err
		}
		info, err := t.actual.Stat(name)
		if err != nil {
			return err
		}
		if err := t.staged.MkdirAll(parentDir(name), 0755); err != nil {
			return err
		}
		if err := afero.WriteFile(t.staged, name, content, info.Mode().Perm()); err != nil {
			return err
		}
	}

	if err := t.staged.Chown(name, uid, gid); err != nil {
		return err
	}
	t.trackPath(name)
	return nil
}

// Chtimes stages a timestamp change.
func (t *TransactFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// If file not in staged, copy from actual
	exists, err := afero.Exists(t.staged, name)
	if err != nil {
		return err
	}
	if !exists {
		content, err := afero.ReadFile(t.actual, name)
		if err != nil {
			return err
		}
		info, err := t.actual.Stat(name)
		if err != nil {
			return err
		}
		if err := t.staged.MkdirAll(parentDir(name), 0755); err != nil {
			return err
		}
		if err := afero.WriteFile(t.staged, name, content, info.Mode().Perm()); err != nil {
			return err
		}
	}

	if err := t.staged.Chtimes(name, atime, mtime); err != nil {
		return err
	}
	t.trackPath(name)
	return nil
}

// -----------------------------------------------------------------------------
// TransactFs: Extension methods (not part of afero.Fs)
// -----------------------------------------------------------------------------
// readFileLocked reads from cow overlay. Caller must hold at least RLock.
func (t *TransactFs) readFileLocked(path string) ([]byte, error) {
	// Check if marked for deletion
	if slices.Contains(t.deletedPaths, path) {
		return nil, os.ErrNotExist
	}

	// Try staged first
	if content, err := afero.ReadFile(t.staged, path); err == nil {
		return content, nil
	}

	// Fallback to actual
	return afero.ReadFile(t.actual, path)
}

// NeedsCommit returns true if there are pending changes to commit.
func (t *TransactFs) NeedsCommit() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	ops, err := t.diffLocked()
	if err != nil {
		return true // Assume changes on error
	}
	return len(ops) > 0
}

// Diff returns the pending operations needed to sync staged to actual.
func (t *TransactFs) Diff() ([]FileOp, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.diffLocked()
}

func (t *TransactFs) diffLocked() ([]FileOp, error) {
	return ComputeDiff(t.staged, t.actual, t.paths, t.deletedPaths)
}

// TrackedPaths returns all paths that have been written or modified.
func (t *TransactFs) TrackedPaths() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return slices.Clone(t.paths)
}

// DeletedPaths returns all paths marked for deletion.
func (t *TransactFs) DeletedPaths() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return slices.Clone(t.deletedPaths)
}

// Commit applies all pending changes via the provided callback.
// On success, staged is reset and tracked paths cleared.
// On failure, staged is preserved for retry.
func (t *TransactFs) Commit(fn CommitFunc) (*CommitResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ops, err := t.diffLocked()
	if err != nil {
		return nil, err
	}

	// Call the callback with context
	ctx := CommitContext{
		BaseFs: t.actual,
		Ops:    ops,
	}

	_, err = fn(ctx)
	if err != nil {
		// On failure, preserve staged state
		return nil, err
	}

	// Success: reset staged state
	t.staged = afero.NewMemMapFs()
	t.paths = nil
	t.deletedPaths = nil

	return &CommitResult{}, nil
}

// trackPath adds a path to the tracked paths list if not already present.
func (t *TransactFs) trackPath(path string) {
	if !slices.Contains(t.paths, path) {
		t.paths = append(t.paths, path)
	}
}

// parentDir returns the parent directory of a path.
func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return "."
}

