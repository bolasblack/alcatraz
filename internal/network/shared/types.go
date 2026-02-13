// Package shared provides shared types and utilities for network implementations.
package shared

import (
	"context"

	"github.com/spf13/afero"

	"github.com/bolasblack/alcatraz/internal/runtime"
	"github.com/bolasblack/alcatraz/internal/util"
)

// NetworkEnv provides dependency injection for filesystem and command execution.
type NetworkEnv struct {
	Fs         afero.Fs                // Filesystem abstraction (may be transactional for staged writes)
	Cmd        util.CommandRunner      // Command execution abstraction
	ProjectDir string                  // Project directory path
	ProjectID  string                  // Project UUID for staleness verification (AGD-014)
	Runtime    runtime.RuntimePlatform // Container runtime platform (injected by CLI)
}

// NewNetworkEnv creates a NetworkEnv with externally provided dependencies.
func NewNetworkEnv(fs afero.Fs, cmd util.CommandRunner, projectDir string, projectID string, rt runtime.RuntimePlatform) *NetworkEnv {
	return &NetworkEnv{
		Fs:         fs,
		Cmd:        cmd,
		ProjectDir: projectDir,
		ProjectID:  projectID,
		Runtime:    rt,
	}
}

// NewTestNetworkEnv returns a NetworkEnv configured for testing with in-memory filesystem
// and a mock command runner.
func NewTestNetworkEnv() *NetworkEnv {
	fs := afero.NewMemMapFs()
	return &NetworkEnv{
		Fs:  fs,
		Cmd: util.NewMockCommandRunner(),
	}
}

// =============================================================================
// Firewall — container-level network isolation (per-container, runtime)
// =============================================================================

// Type represents the firewall backend type.
type Type int

const (
	// TypeNone indicates no firewall is available.
	TypeNone Type = iota
	// TypeNFTables indicates nftables is available (Linux native, macOS via VM).
	TypeNFTables
)

// String returns a human-readable name for the firewall type.
func (t Type) String() string {
	switch t {
	case TypeNFTables:
		return "nftables"
	default:
		return "none"
	}
}

// Firewall manages network isolation rules for containers.
type Firewall interface {
	// ApplyRules applies network isolation with optional allow rules.
	// containerID is used to create an isolated ruleset that can be cleaned up.
	// containerIP is the container's IP address.
	// rules are parsed lan-access entries (allow-listed destinations).
	// If rules is empty, all RFC1918 traffic is blocked.
	// If any rule has AllLAN=true, no blocking is applied.
	// Returns PostCommitAction that MUST be called after TransactFs.Commit().
	ApplyRules(containerID string, containerIP string, rules []LANAccessRule) (*PostCommitAction, error)

	// Cleanup removes all firewall rules for a container.
	// Returns PostCommitAction that MUST be called after TransactFs.Commit().
	Cleanup(containerID string) (*PostCommitAction, error)

	// CleanupStaleFiles removes rule files for projects whose directory no longer exists.
	// Returns the count of cleaned-up files.
	CleanupStaleFiles() (int, error)
}

// =============================================================================
// NetworkHelper — project-level network configuration (per-project, install/up/down)
// =============================================================================

// NetworkHelper manages platform-specific network isolation.
type NetworkHelper interface {
	// Setup configures network for a project during "up".
	// Returns PostCommitAction that MUST be called after TransactFs.Commit().
	Setup(env *NetworkEnv, projectDir string, progress ProgressFunc) (*PostCommitAction, error)

	// Teardown removes network config for a project during "down".
	Teardown(env *NetworkEnv, projectDir string) error

	// HelperStatus checks if helper needs install/update.
	HelperStatus(ctx context.Context, env *NetworkEnv) HelperStatus

	// DetailedStatus returns detailed status for display purposes.
	DetailedStatus(env *NetworkEnv) DetailedStatusInfo

	// InstallHelper installs or updates the network helper.
	// Returns PostCommitAction that MUST be called after TransactFs.Commit().
	InstallHelper(env *NetworkEnv, progress ProgressFunc) (*PostCommitAction, error)

	// UninstallHelper removes the network helper.
	// Returns PostCommitAction that MUST be called after TransactFs.Commit().
	UninstallHelper(env *NetworkEnv, progress ProgressFunc) (*PostCommitAction, error)
}

// PostCommitAction encapsulates actions that must run after TransactFs.Commit().
type PostCommitAction struct {
	Run func(ctx context.Context, progress ProgressFunc) error
}

// HelperStatus reports current state of the network helper.
type HelperStatus struct {
	Installed   bool
	NeedsUpdate bool
}

// DetailedStatusInfo provides implementation-specific status for display.
type DetailedStatusInfo struct {
	// RuleFiles lists the rule files managed by this helper.
	RuleFiles []RuleFileInfo
}

// RuleFileInfo describes a single rule file.
type RuleFileInfo struct {
	Name    string
	Content string
}

// ProgressFunc reports progress during operations.
type ProgressFunc func(format string, args ...any)
