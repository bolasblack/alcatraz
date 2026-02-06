package shared

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/util"
)

// =============================================================================
// DI Contract Tests for NetworkEnv
//
// These tests verify the Dependency Injection contract:
// 1. Constructors store the provided dependencies (don't create new ones)
// 2. Operations use the injected dependencies (not internal ones)
// =============================================================================

// TestNewNetworkEnv_StoresInjectedDeps verifies that NewNetworkEnv stores
// the exact instances passed in, not copies or new instances.
func TestNewNetworkEnv_StoresInjectedDeps(t *testing.T) {
	// Create specific mock instances
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()

	env := NewNetworkEnv(mockFs, mockCmd, "", false)

	// Verify the EXACT instances are stored (pointer equality)
	if env.Fs != mockFs {
		t.Error("NewNetworkEnv must store the exact Fs instance provided, not a copy")
	}
	if env.Cmd != mockCmd {
		t.Error("NewNetworkEnv must store the exact Cmd instance provided, not a copy")
	}
}

// TestNewNetworkEnv_DoesNotCreateOwnDeps verifies that NewNetworkEnv
// doesn't create its own dependencies when provided with nil.
// This would fail if the implementation had fallback logic like:
//
//	if fs == nil { fs = afero.NewOsFs() }
func TestNewNetworkEnv_DoesNotCreateOwnDeps(t *testing.T) {
	// Pass nil explicitly - constructor should store nil, not create defaults
	env := NewNetworkEnv(nil, nil, "", false)

	if env.Fs != nil {
		t.Error("NewNetworkEnv should not create a default Fs when nil is provided")
	}
	if env.Cmd != nil {
		t.Error("NewNetworkEnv should not create a default Cmd when nil is provided")
	}
}

// TestNetworkEnv_FieldsArePublic verifies that Fs and Cmd fields are
// accessible for callers to use. This ensures the struct doesn't hide
// dependencies behind private fields that would require getters.
func TestNetworkEnv_FieldsArePublic(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	mockCmd := util.NewMockCommandRunner()

	env := NewNetworkEnv(mockFs, mockCmd, "", false)

	// These assignments would fail at compile time if fields were private
	_ = afero.Fs(env.Fs)
	_ = util.CommandRunner(env.Cmd)

	// Verify we can use the fields directly
	if env.Fs == nil || env.Cmd == nil {
		t.Error("Fs and Cmd fields must be publicly accessible and non-nil when provided")
	}
}

// TestNewTestNetworkEnv_CreatesMocks verifies that the test helper
// creates real mock instances, not nil values.
func TestNewTestNetworkEnv_CreatesMocks(t *testing.T) {
	env := NewTestNetworkEnv()

	if env.Fs == nil {
		t.Fatal("NewTestNetworkEnv must create a mock Fs")
	}
	if env.Cmd == nil {
		t.Fatal("NewTestNetworkEnv must create a mock Cmd")
	}

	// Verify the Fs is actually a MemMapFs (testable filesystem)
	if _, ok := env.Fs.(*afero.MemMapFs); !ok {
		t.Errorf("NewTestNetworkEnv should create MemMapFs, got %T", env.Fs)
	}

	// Verify the Cmd is actually a MockCommandRunner
	if _, ok := env.Cmd.(*util.MockCommandRunner); !ok {
		t.Errorf("NewTestNetworkEnv should create MockCommandRunner, got %T", env.Cmd)
	}
}

// TestNetworkEnv_InjectedFsIsUsable verifies that operations using the
// injected Fs actually use it (not create a new one internally).
func TestNetworkEnv_InjectedFsIsUsable(t *testing.T) {
	mockFs := afero.NewMemMapFs()
	env := NewNetworkEnv(mockFs, nil, "", false)

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

// TestNetworkEnv_InjectedCmdIsUsable verifies that the injected Cmd
// records calls when used through the env.
func TestNetworkEnv_InjectedCmdIsUsable(t *testing.T) {
	mockCmd := util.NewMockCommandRunner().AllowUnexpected()
	env := NewNetworkEnv(nil, mockCmd, "", false)

	// Run a command through env.Cmd
	_, _ = env.Cmd.Run("test-command", "arg1", "arg2")

	// Verify the mockCmd recorded the call
	if !mockCmd.Called("test-command arg1 arg2") {
		t.Error("env.Cmd and mockCmd should be the same instance - calls should be recorded")
		t.Errorf("Recorded calls: %v", mockCmd.CallKeys())
	}
}
