package sync

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/state"
)

func TestResolveAllInteractive(t *testing.T) {
	conflicts := []ConflictInfo{
		{Path: "file1.txt", LocalState: "modified", ContainerState: "modified", DetectedAt: time.Now()},
		{Path: "file2.txt", LocalState: "created", ContainerState: "deleted", DetectedAt: time.Now()},
		{Path: "file3.txt", LocalState: "deleted", ContainerState: "created", DetectedAt: time.Now()},
	}

	mockState := &state.State{
		ProjectID:     "test-project",
		ContainerName: "test-container",
		Config:        &config.Config{Workdir: "/workspace"},
	}

	tests := []struct {
		name         string
		conflicts    []ConflictInfo
		choices      []ResolveChoice
		promptErr    error // if set, returned on first prompt
		execErr      error
		setupFs      func(fs afero.Fs)
		wantResolved int
		wantSkipped  int
	}{
		{
			name:      "all resolved local",
			conflicts: conflicts[:2],
			choices:   []ResolveChoice{ResolveChoiceLocal, ResolveChoiceLocal},
			setupFs: func(fs afero.Fs) {
				_ = afero.WriteFile(fs, "/project/file1.txt", []byte("a"), 0o644)
				_ = afero.WriteFile(fs, "/project/file2.txt", []byte("b"), 0o644)
			},
			wantResolved: 2,
			wantSkipped:  0,
		},
		{
			name:      "all resolved container",
			conflicts: conflicts[:2],
			choices:   []ResolveChoice{ResolveChoiceContainer, ResolveChoiceContainer},
			setupFs: func(fs afero.Fs) {
				_ = afero.WriteFile(fs, "/project/file1.txt", []byte("a"), 0o644)
				_ = afero.WriteFile(fs, "/project/file2.txt", []byte("b"), 0o644)
			},
			wantResolved: 2,
			wantSkipped:  0,
		},
		{
			name:         "all skipped",
			conflicts:    conflicts[:2],
			choices:      []ResolveChoice{ResolveChoiceSkip, ResolveChoiceSkip},
			setupFs:      func(fs afero.Fs) {},
			wantResolved: 0,
			wantSkipped:  2,
		},
		{
			name:      "mixed choices",
			conflicts: conflicts,
			choices:   []ResolveChoice{ResolveChoiceLocal, ResolveChoiceSkip, ResolveChoiceContainer},
			setupFs: func(fs afero.Fs) {
				_ = afero.WriteFile(fs, "/project/file3.txt", []byte("c"), 0o644)
			},
			wantResolved: 2,
			wantSkipped:  1,
		},
		{
			name:         "user cancel on first prompt",
			conflicts:    conflicts[:2],
			promptErr:    fmt.Errorf("user cancelled"),
			setupFs:      func(fs afero.Fs) {},
			wantResolved: 0,
			wantSkipped:  0,
		},
		{
			name:         "executor error continues to next",
			conflicts:    conflicts[:2],
			choices:      []ResolveChoice{ResolveChoiceLocal, ResolveChoiceLocal},
			execErr:      fmt.Errorf("exec failed"),
			setupFs:      func(fs afero.Fs) {},
			wantResolved: 0,
			wantSkipped:  0,
		},
		{
			name:         "zero conflicts",
			conflicts:    []ConflictInfo{},
			setupFs:      func(fs afero.Fs) {},
			wantResolved: 0,
			wantSkipped:  0,
		},
		{
			name:      "container resolve error continues to next",
			conflicts: conflicts[:2],
			choices:   []ResolveChoice{ResolveChoiceContainer, ResolveChoiceContainer},
			setupFs: func(fs afero.Fs) {
				// Only create file2, file1 is missing so Remove will fail
				_ = afero.WriteFile(fs, "/project/file2.txt", []byte("b"), 0o644)
			},
			wantResolved: 1,
			wantSkipped:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tt.setupFs(fs)

			var buf bytes.Buffer
			env := &SyncEnv{
				Fs:       fs,
				Sessions: &mockSyncSessionClient{},
			}

			executor := &mockExecutor{err: tt.execErr}

			callIdx := 0
			promptFn := func(conflict ConflictInfo, index, total int) (ResolveChoice, error) {
				if tt.promptErr != nil {
					return "", tt.promptErr
				}
				choice := tt.choices[callIdx]
				callIdx++
				return choice, nil
			}

			result, err := ResolveAllInteractive(ResolveParams{
				Ctx:         context.Background(),
				Env:         env,
				Executor:    executor,
				State:       mockState,
				ProjectRoot: "/project",
				Conflicts:   tt.conflicts,
				PromptFn:    promptFn,
				W:           &buf,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Resolved != tt.wantResolved {
				t.Errorf("resolved: got %d, want %d", result.Resolved, tt.wantResolved)
			}
			if result.Skipped != tt.wantSkipped {
				t.Errorf("skipped: got %d, want %d", result.Skipped, tt.wantSkipped)
			}
		})
	}
}

// TestResolveAllInteractive_PromptErrorMidWay tests that when the prompt
// returns an error partway through, earlier resolved/skipped counts are preserved.
func TestResolveAllInteractive_PromptErrorMidWay(t *testing.T) {
	conflicts := []ConflictInfo{
		{Path: "file1.txt", LocalState: "modified", ContainerState: "modified", DetectedAt: time.Now()},
		{Path: "file2.txt", LocalState: "created", ContainerState: "deleted", DetectedAt: time.Now()},
		{Path: "file3.txt", LocalState: "deleted", ContainerState: "created", DetectedAt: time.Now()},
	}

	mockState := &state.State{
		ProjectID:     "test-project",
		ContainerName: "test-container",
		Config:        &config.Config{Workdir: "/workspace"},
	}

	var buf bytes.Buffer
	fs := afero.NewMemMapFs()
	env := &SyncEnv{Fs: fs, Sessions: &mockSyncSessionClient{}}

	callIdx := 0
	promptFn := func(conflict ConflictInfo, index, total int) (ResolveChoice, error) {
		callIdx++
		switch callIdx {
		case 1:
			return ResolveChoiceLocal, nil // succeeds (executor mock succeeds)
		case 2:
			return ResolveChoiceSkip, nil
		default:
			return "", fmt.Errorf("user cancelled")
		}
	}

	result, err := ResolveAllInteractive(ResolveParams{
		Ctx:         context.Background(),
		Env:         env,
		Executor:    &mockExecutor{},
		State:       mockState,
		ProjectRoot: "/project",
		Conflicts:   conflicts,
		PromptFn:    promptFn,
		W:           &buf,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Resolved != 1 {
		t.Errorf("resolved: got %d, want 1", result.Resolved)
	}
	if result.Skipped != 1 {
		t.Errorf("skipped: got %d, want 1", result.Skipped)
	}
	// The abort message should include the counts
	if !strings.Contains(buf.String(), "Aborted. 1 resolved, 1 skipped") {
		t.Errorf("output should contain abort message with counts, got: %s", buf.String())
	}
}

// TestResolveAllInteractive_FirstSucceedsSecondFails tests that when the first
// resolution succeeds but the second fails, the result reflects partial success.
func TestResolveAllInteractive_FirstSucceedsSecondFails(t *testing.T) {
	conflicts := []ConflictInfo{
		{Path: "file1.txt", LocalState: "modified", ContainerState: "modified", DetectedAt: time.Now()},
		{Path: "file2.txt", LocalState: "created", ContainerState: "deleted", DetectedAt: time.Now()},
	}

	mockState := &state.State{
		ProjectID:     "test-project",
		ContainerName: "test-container",
		Config:        &config.Config{Workdir: "/workspace"},
	}

	var buf bytes.Buffer
	fs := afero.NewMemMapFs()
	env := &SyncEnv{Fs: fs, Sessions: &mockSyncSessionClient{}}

	callCount := 0
	executor := &callCountExecutor{
		errOnCall: 2, // fail on 2nd call
		err:       fmt.Errorf("container unreachable"),
	}

	promptFn := func(conflict ConflictInfo, index, total int) (ResolveChoice, error) {
		callCount++
		return ResolveChoiceLocal, nil
	}

	result, err := ResolveAllInteractive(ResolveParams{
		Ctx:         context.Background(),
		Env:         env,
		Executor:    executor,
		State:       mockState,
		ProjectRoot: "/project",
		Conflicts:   conflicts,
		PromptFn:    promptFn,
		W:           &buf,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Resolved != 1 {
		t.Errorf("resolved: got %d, want 1", result.Resolved)
	}
	// Output should contain error message for the failed resolution
	if !strings.Contains(buf.String(), "Error:") {
		t.Errorf("output should contain error message, got: %s", buf.String())
	}
}

// TestResolveAllInteractive_ZeroConflictsOutput verifies output for empty conflicts.
func TestResolveAllInteractive_ZeroConflictsOutput(t *testing.T) {
	mockState := &state.State{
		ProjectID:     "test-project",
		ContainerName: "test-container",
		Config:        &config.Config{Workdir: "/workspace"},
	}

	var buf bytes.Buffer
	fs := afero.NewMemMapFs()
	env := &SyncEnv{Fs: fs, Sessions: &mockSyncSessionClient{}}

	result, err := ResolveAllInteractive(ResolveParams{
		Ctx:         context.Background(),
		Env:         env,
		Executor:    &mockExecutor{},
		State:       mockState,
		ProjectRoot: "/project",
		Conflicts:   []ConflictInfo{},
		PromptFn: func(conflict ConflictInfo, index, total int) (ResolveChoice, error) {
			t.Fatal("prompt should not be called with zero conflicts")
			return "", nil
		},
		W: &buf,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Resolved != 0 || result.Skipped != 0 {
		t.Errorf("expected 0/0, got resolved=%d skipped=%d", result.Resolved, result.Skipped)
	}
	if !strings.Contains(buf.String(), "Done: 0 resolved, 0 skipped") {
		t.Errorf("expected done message, got: %s", buf.String())
	}
}

// TestFlushProjectSyncs tests the FlushProjectSyncs helper.
func TestFlushProjectSyncs(t *testing.T) {
	t.Run("nil sessions does not panic", func(t *testing.T) {
		FlushProjectSyncs(context.Background(), nil, "test-project")
	})

	t.Run("list error is silently ignored", func(t *testing.T) {
		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				return nil, fmt.Errorf("connection lost")
			},
		}
		FlushProjectSyncs(context.Background(), mock, "test-project")
	})

	t.Run("flush error is silently ignored per session", func(t *testing.T) {
		flushed := []string{}
		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				return []string{"alca-proj-0", "alca-proj-1"}, nil
			},
			flushSyncSessionFn: func(name string) error {
				flushed = append(flushed, name)
				if name == "alca-proj-0" {
					return fmt.Errorf("flush failed")
				}
				return nil
			},
		}
		FlushProjectSyncs(context.Background(), mock, "proj")
		// Both sessions should be attempted even if the first fails
		if len(flushed) != 2 {
			t.Errorf("expected 2 flush attempts, got %d", len(flushed))
		}
	})

	t.Run("flushes all matching sessions", func(t *testing.T) {
		flushed := []string{}
		mock := &mockSyncSessionClient{
			listSyncSessionsFn: func(namePrefix string) ([]string, error) {
				return []string{"alca-proj-0", "alca-proj-1"}, nil
			},
			flushSyncSessionFn: func(name string) error {
				flushed = append(flushed, name)
				return nil
			},
		}
		FlushProjectSyncs(context.Background(), mock, "proj")
		if len(flushed) != 2 {
			t.Errorf("expected 2 flush calls, got %d", len(flushed))
		}
	})
}

// callCountExecutor fails on a specific call number.
type callCountExecutor struct {
	callCount int
	errOnCall int
	err       error
}

var _ ContainerExecutor = (*callCountExecutor)(nil)

func (e *callCountExecutor) ExecInContainer(_ context.Context, containerID string, cmd []string) error {
	e.callCount++
	if e.callCount == e.errOnCall {
		return e.err
	}
	return nil
}
