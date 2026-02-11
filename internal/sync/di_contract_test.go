package sync

import (
	"context"
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// DI Contract Tests for SyncEnv
//
// These tests verify the Dependency Injection contract behaviorally:
// Operations use the injected dependencies (not internally-created ones).
// =============================================================================

// TestSyncEnv_InjectedFsIsUsable verifies that operations using the
// injected Fs actually use it (not create a new one internally).
func TestSyncEnv_InjectedFsIsUsable(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	env := NewSyncEnv(mockFs, nil, nil)

	// Write a file using the env's Fs
	testContent := []byte("test content")
	err := afero.WriteFile(env.Fs, "/test/file.txt", testContent, 0644)
	if err != nil {
		t.Fatalf("Failed to write via env.Fs: %v", err)
	}

	// Read it back from the original mockFs to verify it's the same instance
	content, err := afero.ReadFile(mockFs, "/test/file.txt")
	if err != nil {
		t.Fatalf("Failed to read from mockFs: %v", err)
	}

	if string(content) != string(testContent) {
		t.Error("env.Fs and mockFs should be the same instance - writes should be visible from both")
	}
}

// TestSyncEnv_InjectedCmdIsUsable verifies that the injected Cmd
// records calls when used through the env.
func TestSyncEnv_InjectedCmdIsUsable(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := NewSyncEnv(nil, mockCmd, nil)

	// Run a command through env.Cmd
	_, _ = env.Cmd.Run("test-command", "arg1", "arg2")

	// Verify the mockCmd recorded the call
	if !mockCmd.Called("test-command arg1 arg2") {
		t.Error("env.Cmd and mockCmd should be the same instance - calls should be recorded")
		t.Errorf("Recorded calls: %v", mockCmd.CallKeys())
	}
}

// TestSyncEnv_InjectedSessionsIsUsable verifies that the injected Sessions
// is used by operations that depend on it.
func TestSyncEnv_InjectedSessionsIsUsable(t *testing.T) {
	called := false
	mock := &mockSyncSessionClient{
		listSessionJSONFn: func(sessionName string) ([]byte, error) {
			called = true
			return []byte(`[]`), nil
		},
	}
	env := NewSyncEnv(nil, nil, mock)

	_, err := env.DetectConflicts(context.Background(), "test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("env.Sessions should be the injected mock - DetectConflicts should call it")
	}
}
