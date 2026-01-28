// Package transact provides transactional file system operations.
// This file contains acceptance tests for the ideal TransactFs model.
//
// IDEAL MODEL REQUIREMENTS:
// 1. CopyOnWriteFs semantics: reads check staged first, then actual; writes go to staged
// 2. Commit(fn): accepts a callback that receives []FileOp and performs actual writes
//    - Callback success: staged resets to empty, generation increases
//    - Callback failure: staged unchanged, stays in "pending commit" state
// 3. After Commit, ALL old handles (regardless of when opened) see the new state
// 4. Remove follows Unix semantics: old handles retain content, new opens fail
// 5. Commit blocks all reads/writes during execution (exclusive lock)
//    - Callback MUST NOT call tfs methods (will deadlock)
//
// These tests define the expected behavior. Implementation must pass all tests.
package transact

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: CommitContext, CommitOpsResult, CommitResult, CommitFunc are defined in fs.go
// Commit signature is:
//   func (t *TransactFs) Commit(fn CommitFunc) (*CommitResult, error)

// mockCommitFunc returns a commit callback that records the ops it receives
// and optionally returns an error.
func mockCommitFunc(recordedOps *[]FileOp, returnErr error) CommitFunc {
	return func(ctx CommitContext) (*CommitOpsResult, error) {
		*recordedOps = ctx.Ops
		return nil, returnErr
	}
}

// successCommitFunc returns a commit callback that writes ops to actual fs.
func successCommitFunc() CommitFunc {
	return func(ctx CommitContext) (*CommitOpsResult, error) {
		for _, op := range ctx.Ops {
			switch op.Op {
			case OpCreate, OpUpdate:
				if err := ctx.BaseFs.MkdirAll(parentDir(op.Path), 0755); err != nil {
					return nil, err
				}
				if err := afero.WriteFile(ctx.BaseFs, op.Path, op.Content, op.Mode); err != nil {
					return nil, err
				}
			case OpChmod:
				if err := ctx.BaseFs.Chmod(op.Path, op.Mode); err != nil {
					return nil, err
				}
			case OpDelete:
				if err := ctx.BaseFs.Remove(op.Path); err != nil && !os.IsNotExist(err) {
					return nil, err
				}
			}
		}
		return nil, nil
	}
}

// noopCommitFunc returns a commit callback that does nothing (for testing commit logic only).
func noopCommitFunc() CommitFunc {
	return func(ctx CommitContext) (*CommitOpsResult, error) {
		return nil, nil
	}
}

// =============================================================================
// Basic CopyOnWriteFs Semantics (before Commit)
// =============================================================================

func TestIdeal_ReadFallbackToActual(t *testing.T) {
	// When file exists only in actual, read should return actual content
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("actual-content"), 0644)

	tfs := New(WithActualFs(actual))

	content, err := afero.ReadFile(tfs,"/test")
	require.NoError(t, err)
	assert.Equal(t, "actual-content", string(content))
}

func TestIdeal_ReadPrefersStaged(t *testing.T) {
	// When file exists in both staged and actual, read should return staged content
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("actual-content"), 0644)

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/test", []byte("staged-content"), 0644)

	content, err := afero.ReadFile(tfs,"/test")
	require.NoError(t, err)
	assert.Equal(t, "staged-content", string(content))
}

func TestIdeal_WriteGoesToStaged(t *testing.T) {
	// Write should not modify actual until Commit
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/test", []byte("modified"), 0644)

	// actual should be unchanged
	actualContent, _ := afero.ReadFile(actual, "/test")
	assert.Equal(t, "original", string(actualContent))

	// tfs should return staged content
	tfsContent, _ := afero.ReadFile(tfs,"/test")
	assert.Equal(t, "modified", string(tfsContent))
}

func TestIdeal_ReadAfterWrite_BeforeCommit(t *testing.T) {
	// Read-after-write (before commit) should return written content
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Open handle
	f, err := tfs.Open("/test")
	require.NoError(t, err)
	defer f.Close()

	// Read original
	content, _ := io.ReadAll(f)
	assert.Equal(t, "original", string(content))

	// Write new content
	afero.WriteFile(tfs,"/test", []byte("new-content"), 0644)

	// Read via same handle should see new content (CopyOnWrite semantics)
	f.Seek(0, io.SeekStart)
	content, _ = io.ReadAll(f)
	assert.Equal(t, "new-content", string(content))
}

// =============================================================================
// Commit Behavior
// =============================================================================

func TestIdeal_CommitWritesToActual(t *testing.T) {
	// After Commit, actual should contain staged changes
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/test", []byte("committed"), 0644)

	_, err := tfs.Commit(successCommitFunc())
	require.NoError(t, err)

	// actual should now have new content
	actualContent, _ := afero.ReadFile(actual, "/test")
	assert.Equal(t, "committed", string(actualContent))
}

func TestIdeal_CommitResetsStaged(t *testing.T) {
	// After Commit, staged should be empty (fresh state)
	actual := afero.NewMemMapFs()

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/test", []byte("content"), 0644)

	tfs.Commit(successCommitFunc())

	// NeedsCommit should return false (no pending changes)
	assert.False(t, tfs.NeedsCommit())

	// TrackedPaths should be empty
	assert.Empty(t, tfs.TrackedPaths())
}

// =============================================================================
// Commit Callback Behavior
// =============================================================================

func TestIdeal_CommitCallback_ReceivesCorrectOps(t *testing.T) {
	// Commit callback should receive the correct FileOps
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/existing", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/existing", []byte("modified"), 0644)
	afero.WriteFile(tfs,"/newfile", []byte("new"), 0644)
	tfs.Remove("/todelete")

	var receivedOps []FileOp
	_, err := tfs.Commit(mockCommitFunc(&receivedOps, nil))
	require.NoError(t, err)

	// Should have received ops for all changes
	assert.NotEmpty(t, receivedOps)

	// Verify we can find expected operations
	var hasUpdate, hasCreate bool
	for _, op := range receivedOps {
		if op.Path == "/existing" && op.Op == OpUpdate {
			hasUpdate = true
			assert.Equal(t, []byte("modified"), op.Content)
		}
		if op.Path == "/newfile" && op.Op == OpCreate {
			hasCreate = true
			assert.Equal(t, []byte("new"), op.Content)
		}
	}
	assert.True(t, hasUpdate, "should have update op for /existing")
	assert.True(t, hasCreate, "should have create op for /newfile")
}

func TestIdeal_CommitCallback_FailurePreservesStaged(t *testing.T) {
	// If callback returns error, staged should be preserved
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/test", []byte("staged-content"), 0644)

	// Commit with failing callback
	expectedErr := errors.New("commit failed")
	_, err := tfs.Commit(func(ctx CommitContext) (*CommitOpsResult, error) {
		return nil, expectedErr
	})

	// Should return the error
	assert.Equal(t, expectedErr, err)

	// Staged should be preserved - can still read staged content
	content, _ := afero.ReadFile(tfs,"/test")
	assert.Equal(t, "staged-content", string(content))

	// NeedsCommit should still be true
	assert.True(t, tfs.NeedsCommit())

	// TrackedPaths should still contain the path
	assert.Contains(t, tfs.TrackedPaths(), "/test")
}

func TestIdeal_CommitCallback_FailureAllowsRetry(t *testing.T) {
	// After failed commit, should be able to retry
	actual := afero.NewMemMapFs()

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/test", []byte("content"), 0644)

	// First commit fails
	attemptCount := 0
	_, err := tfs.Commit(func(ctx CommitContext) (*CommitOpsResult, error) {
		attemptCount++
		if attemptCount == 1 {
			return nil, errors.New("first attempt failed")
		}
		// On success, write to actual
		for _, op := range ctx.Ops {
			if op.Op == OpCreate || op.Op == OpUpdate {
				afero.WriteFile(ctx.BaseFs, op.Path, op.Content, op.Mode)
			}
		}
		return nil, nil
	})
	assert.Error(t, err)

	// Second commit succeeds
	_, err = tfs.Commit(func(ctx CommitContext) (*CommitOpsResult, error) {
		attemptCount++
		for _, op := range ctx.Ops {
			if op.Op == OpCreate || op.Op == OpUpdate {
				afero.WriteFile(ctx.BaseFs, op.Path, op.Content, op.Mode)
			}
		}
		return nil, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, attemptCount) // 1 failed + 1 succeeded

	// Now staged should be reset
	assert.False(t, tfs.NeedsCommit())
}

func TestIdeal_CommitCallback_PartialWriteOnFailure(t *testing.T) {
	// If callback partially writes to actual then fails,
	// staged should still be preserved for potential retry
	actualFs := afero.NewMemMapFs()
	afero.WriteFile(actualFs, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actualFs))
	afero.WriteFile(tfs,"/test", []byte("new-content"), 0644)

	// Callback that writes to actual but then fails
	_, err := tfs.Commit(func(ctx CommitContext) (*CommitOpsResult, error) {
		// Simulate partial write to actual
		afero.WriteFile(ctx.BaseFs, "/test", []byte("partial"), 0644)
		return nil, errors.New("failed after partial write")
	})
	assert.Error(t, err)

	// Staged should still be preserved
	content, _ := afero.ReadFile(tfs,"/test")
	assert.Equal(t, "new-content", string(content))

	// actual may have partial write (this is expected - caller's responsibility)
	actualContent, _ := afero.ReadFile(actualFs, "/test")
	assert.Equal(t, "partial", string(actualContent))
}

func TestIdeal_CommitCallback_ReceivesContext(t *testing.T) {
	// Callback should receive CommitContext with BaseFs and Ops
	actualFs := afero.NewMemMapFs()
	afero.WriteFile(actualFs, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actualFs))
	afero.WriteFile(tfs,"/test", []byte("new"), 0644)

	var receivedCtx CommitContext
	tfs.Commit(func(ctx CommitContext) (*CommitOpsResult, error) {
		receivedCtx = ctx
		// Write via received BaseFs
		afero.WriteFile(ctx.BaseFs, "/test", []byte("written-via-callback"), 0644)
		return nil, nil
	})

	// Should have received the same actual fs
	assert.Equal(t, actualFs, receivedCtx.BaseFs)

	// Should have received ops
	assert.NotEmpty(t, receivedCtx.Ops)

	// Write should have gone to actual
	content, _ := afero.ReadFile(actualFs, "/test")
	assert.Equal(t, "written-via-callback", string(content))
}

func TestIdeal_CommitCallback_ReturnsOpsResult(t *testing.T) {
	// Commit callback can return *CommitOpsResult
	actual := afero.NewMemMapFs()

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/test", []byte("content"), 0644)

	result, err := tfs.Commit(func(ctx CommitContext) (*CommitOpsResult, error) {
		for _, op := range ctx.Ops {
			if op.Op == OpCreate || op.Op == OpUpdate {
				afero.WriteFile(ctx.BaseFs, op.Path, op.Content, op.Mode)
			}
		}
		return &CommitOpsResult{}, nil
	})

	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestIdeal_CommitCallback_ReturnsNilOpsResult(t *testing.T) {
	// Commit callback can return nil as OpsResult
	actual := afero.NewMemMapFs()

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/test", []byte("content"), 0644)

	result, err := tfs.Commit(func(ctx CommitContext) (*CommitOpsResult, error) {
		for _, op := range ctx.Ops {
			if op.Op == OpCreate || op.Op == OpUpdate {
				afero.WriteFile(ctx.BaseFs, op.Path, op.Content, op.Mode)
			}
		}
		return nil, nil
	})

	require.NoError(t, err)
	assert.NotNil(t, result) // CommitResult is always returned
}

func TestIdeal_CommitNewFile(t *testing.T) {
	// Commit should create new files in actual
	actual := afero.NewMemMapFs()

	tfs := New(WithActualFs(actual))
	afero.WriteFile(tfs,"/newfile", []byte("new"), 0644)

	tfs.Commit(successCommitFunc())

	// File should exist in actual
	content, err := afero.ReadFile(actual, "/newfile")
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
}

func TestIdeal_CommitDelete(t *testing.T) {
	// Commit should delete files from actual
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("to-delete"), 0644)

	tfs := New(WithActualFs(actual))
	tfs.Remove("/test")

	tfs.Commit(successCommitFunc())

	// File should not exist in actual
	_, err := actual.Stat("/test")
	assert.True(t, os.IsNotExist(err))
}

// =============================================================================
// Old Handle Behavior After Write -> Commit (CRITICAL)
// =============================================================================

func TestIdeal_OldHandle_OpenedBeforeWrite_SeesNewContentAfterCommit(t *testing.T) {
	// Handle opened BEFORE write should see new content after Commit
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Open handle BEFORE write
	f, err := tfs.Open("/test")
	require.NoError(t, err)
	defer f.Close()

	// Write and Commit
	afero.WriteFile(tfs,"/test", []byte("committed"), 0644)
	tfs.Commit(successCommitFunc())

	// Old handle should see committed content
	f.Seek(0, io.SeekStart)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "committed", string(content))
}

func TestIdeal_OldHandle_OpenedAfterWrite_SeesNewContentAfterCommit(t *testing.T) {
	// Handle opened AFTER write should see new content after Commit
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Write first
	afero.WriteFile(tfs,"/test", []byte("staged"), 0644)

	// Open handle AFTER write
	f, err := tfs.Open("/test")
	require.NoError(t, err)
	defer f.Close()

	// Verify it sees staged content before commit
	content, _ := io.ReadAll(f)
	assert.Equal(t, "staged", string(content))

	// Commit
	tfs.Commit(successCommitFunc())

	// Old handle should still see committed content (same as staged in this case)
	f.Seek(0, io.SeekStart)
	content, err = io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "staged", string(content))
}

func TestIdeal_OldHandle_MultipleHandles_AllSeeNewContentAfterCommit(t *testing.T) {
	// Multiple handles opened at different times should all see new content after Commit
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Handle 1: opened before write
	f1, _ := tfs.Open("/test")
	defer f1.Close()

	// Write
	afero.WriteFile(tfs,"/test", []byte("staged"), 0644)

	// Handle 2: opened after write
	f2, _ := tfs.Open("/test")
	defer f2.Close()

	// Commit
	tfs.Commit(successCommitFunc())

	// Both handles should see committed content
	f1.Seek(0, io.SeekStart)
	c1, _ := io.ReadAll(f1)
	assert.Equal(t, "staged", string(c1), "f1 (opened before write) should see committed content")

	f2.Seek(0, io.SeekStart)
	c2, _ := io.ReadAll(f2)
	assert.Equal(t, "staged", string(c2), "f2 (opened after write) should see committed content")
}

// =============================================================================
// Old Handle Behavior After Remove -> Commit (Unix Semantics)
// =============================================================================

func TestIdeal_OldHandle_AfterRemoveCommit_RetainsContent(t *testing.T) {
	// Old handle should retain content after file is removed and committed (Unix semantics)
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Open handle before remove
	f, err := tfs.Open("/test")
	require.NoError(t, err)
	defer f.Close()

	// Remove and Commit
	tfs.Remove("/test")
	tfs.Commit(successCommitFunc())

	// Old handle should still read original content
	f.Seek(0, io.SeekStart)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))
}

func TestIdeal_NewHandle_AfterRemoveCommit_Fails(t *testing.T) {
	// New handle after remove+commit should fail (file not found)
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Remove and Commit
	tfs.Remove("/test")
	tfs.Commit(successCommitFunc())

	// New open should fail
	_, err := tfs.Open("/test")
	assert.Error(t, err)
}

// =============================================================================
// Write -> Remove -> Commit Scenario (CRITICAL)
// =============================================================================

func TestIdeal_OldHandle_WriteRemoveCommit_SeesWrittenContent(t *testing.T) {
	// open -> write -> remove -> commit -> read
	// Read should return the written content (last write before delete)
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Open handle
	f, err := tfs.Open("/test")
	require.NoError(t, err)
	defer f.Close()

	// Write new content
	afero.WriteFile(tfs,"/test", []byte("written-before-delete"), 0644)

	// Remove
	tfs.Remove("/test")

	// Commit (executes delete)
	tfs.Commit(successCommitFunc())

	// Read should return written content (not original, not error)
	f.Seek(0, io.SeekStart)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "written-before-delete", string(content))
}

func TestIdeal_OldHandle_WriteRemoveCommit_HandleOpenedAfterWrite(t *testing.T) {
	// Handle opened after write, then remove+commit
	// Should see written content
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Write new content
	afero.WriteFile(tfs,"/test", []byte("written"), 0644)

	// Open handle after write
	f, err := tfs.Open("/test")
	require.NoError(t, err)
	defer f.Close()

	// Remove and Commit
	tfs.Remove("/test")
	tfs.Commit(successCommitFunc())

	// Read should return written content
	f.Seek(0, io.SeekStart)
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, "written", string(content))
}

// =============================================================================
// Position Preservation
// =============================================================================

func TestIdeal_OldHandle_PositionPreservedAfterCommit(t *testing.T) {
	// File position should be preserved after Commit
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("0123456789"), 0644)

	tfs := New(WithActualFs(actual))

	// Open and seek to position 5
	f, _ := tfs.Open("/test")
	defer f.Close()
	f.Seek(5, io.SeekStart)

	// Write longer content and commit
	afero.WriteFile(tfs,"/test", []byte("ABCDEFGHIJKLMNOP"), 0644)
	tfs.Commit(successCommitFunc())

	// Read from position 5 should return "FGHIJKLMNOP"
	content, _ := io.ReadAll(f)
	assert.Equal(t, "FGHIJKLMNOP", string(content))
}

func TestIdeal_OldHandle_PositionBeyondNewLength(t *testing.T) {
	// If position is beyond new file length after Commit, read should return EOF
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("0123456789"), 0644)

	tfs := New(WithActualFs(actual))

	// Open and seek to position 8
	f, _ := tfs.Open("/test")
	defer f.Close()
	f.Seek(8, io.SeekStart)

	// Write shorter content and commit
	afero.WriteFile(tfs,"/test", []byte("ABC"), 0644)
	tfs.Commit(successCommitFunc())

	// Read should return EOF (position 8 is beyond length 3)
	// Note: io.ReadAll converts EOF to nil, so test with direct Read
	buf := make([]byte, 10)
	n, err := f.Read(buf)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

// =============================================================================
// Multiple Commits
// =============================================================================

func TestIdeal_MultipleCommits(t *testing.T) {
	// TransactFs should work correctly across multiple commits
	actual := afero.NewMemMapFs()

	tfs := New(WithActualFs(actual))

	// First cycle
	afero.WriteFile(tfs,"/test", []byte("commit1"), 0644)
	tfs.Commit(successCommitFunc())

	content, _ := afero.ReadFile(tfs,"/test")
	assert.Equal(t, "commit1", string(content))

	// Second cycle
	afero.WriteFile(tfs,"/test", []byte("commit2"), 0644)
	tfs.Commit(successCommitFunc())

	content, _ = afero.ReadFile(tfs,"/test")
	assert.Equal(t, "commit2", string(content))

	// Third cycle with handle
	f, _ := tfs.Open("/test")
	defer f.Close()

	afero.WriteFile(tfs,"/test", []byte("commit3"), 0644)
	tfs.Commit(successCommitFunc())

	f.Seek(0, io.SeekStart)
	c, _ := io.ReadAll(f)
	assert.Equal(t, "commit3", string(c))
}

func TestIdeal_HandleAcrossMultipleCommits(t *testing.T) {
	// Single handle should see all changes across multiple commits
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	f, _ := tfs.Open("/test")
	defer f.Close()

	// First commit
	afero.WriteFile(tfs,"/test", []byte("commit1"), 0644)
	tfs.Commit(successCommitFunc())

	f.Seek(0, io.SeekStart)
	c, _ := io.ReadAll(f)
	assert.Equal(t, "commit1", string(c))

	// Second commit
	afero.WriteFile(tfs,"/test", []byte("commit2"), 0644)
	tfs.Commit(successCommitFunc())

	f.Seek(0, io.SeekStart)
	c, _ = io.ReadAll(f)
	assert.Equal(t, "commit2", string(c))
}

// =============================================================================
// OpenFile with Write Flags
// =============================================================================

func TestIdeal_OpenFileWrite_ContentVisibleViaRead(t *testing.T) {
	// Content written via OpenFile should be visible to other reads
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Open for write
	fw, err := tfs.OpenFile("/test", os.O_RDWR|os.O_TRUNC, 0644)
	require.NoError(t, err)
	fw.Write([]byte("written-via-handle"))
	fw.Close()

	// Read via different method
	content, _ := afero.ReadFile(tfs,"/test")
	assert.Equal(t, "written-via-handle", string(content))
}

func TestIdeal_OpenFileWrite_CommitPersists(t *testing.T) {
	// Content written via OpenFile should be committed
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	// Open for write
	fw, _ := tfs.OpenFile("/test", os.O_RDWR|os.O_TRUNC, 0644)
	fw.Write([]byte("to-commit"))
	fw.Close()

	// Commit
	tfs.Commit(successCommitFunc())

	// actual should have new content
	actualContent, _ := afero.ReadFile(actual, "/test")
	assert.Equal(t, "to-commit", string(actualContent))
}

// =============================================================================
// Chmod Behavior
// =============================================================================

func TestIdeal_Chmod_CommitChangesPermissions(t *testing.T) {
	// Chmod should be staged and committed
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("content"), 0644)

	tfs := New(WithActualFs(actual))

	// Change permissions
	tfs.Chmod("/test", 0755)

	// Before commit, actual unchanged
	info, _ := actual.Stat("/test")
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())

	// Commit
	tfs.Commit(successCommitFunc())

	// After commit, actual has new permissions
	info, _ = actual.Stat("/test")
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

// =============================================================================
// Edge Cases
// =============================================================================

func TestIdeal_CommitNoChanges(t *testing.T) {
	// Commit with no changes should be no-op
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))

	_, err := tfs.Commit(successCommitFunc())
	assert.NoError(t, err)

	// actual unchanged
	content, _ := afero.ReadFile(actual, "/test")
	assert.Equal(t, "original", string(content))
}

func TestIdeal_ReadNonExistentFile(t *testing.T) {
	// Reading non-existent file should fail
	actual := afero.NewMemMapFs()
	tfs := New(WithActualFs(actual))

	_, err := afero.ReadFile(tfs,"/nonexistent")
	assert.Error(t, err)
}

func TestIdeal_WriteNewFile_ReadBeforeCommit(t *testing.T) {
	// New file should be readable before commit (from staged)
	actual := afero.NewMemMapFs()
	tfs := New(WithActualFs(actual))

	afero.WriteFile(tfs,"/newfile", []byte("new-content"), 0644)

	content, err := afero.ReadFile(tfs,"/newfile")
	require.NoError(t, err)
	assert.Equal(t, "new-content", string(content))
}

func TestIdeal_RemoveBeforeCommit_ReadFails(t *testing.T) {
	// After staging a remove (before commit), read should fail
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))
	tfs.Remove("/test")

	// Read should fail (file marked for deletion)
	_, err := afero.ReadFile(tfs,"/test")
	assert.Error(t, err)
}

func TestIdeal_WriteAfterRemove_Resurrects(t *testing.T) {
	// Writing after remove should "resurrect" the file
	actual := afero.NewMemMapFs()
	afero.WriteFile(actual, "/test", []byte("original"), 0644)

	tfs := New(WithActualFs(actual))
	tfs.Remove("/test")
	afero.WriteFile(tfs,"/test", []byte("resurrected"), 0644)

	content, err := afero.ReadFile(tfs,"/test")
	require.NoError(t, err)
	assert.Equal(t, "resurrected", string(content))

	// After commit, file should exist with new content
	tfs.Commit(successCommitFunc())
	actualContent, _ := afero.ReadFile(actual, "/test")
	assert.Equal(t, "resurrected", string(actualContent))
}
