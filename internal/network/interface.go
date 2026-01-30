// Package network provides network configuration helpers for Alcatraz.
// See AGD-023 for LAN access design decisions.
package network

import "github.com/bolasblack/alcatraz/internal/util"

// NetworkHelper manages platform-specific network isolation.
type NetworkHelper interface {
	// Setup configures network for a project during "up".
	// Returns PostCommitAction that MUST be called after TransactFs.Commit().
	Setup(env *util.Env, projectDir string, progress ProgressFunc) (*PostCommitAction, error)

	// Teardown removes network config for a project during "down".
	Teardown(env *util.Env, projectDir string) error

	// HelperStatus checks if helper needs install/update.
	HelperStatus(env *util.Env) HelperStatus

	// DetailedStatus returns detailed status for display purposes.
	// Encapsulates implementation-specific details (file paths, daemon state)
	// so callers don't need to know about pf anchors, iptables, etc.
	DetailedStatus(env *util.Env) DetailedStatusInfo

	// InstallHelper installs or updates the network helper.
	// Returns PostCommitAction that MUST be called after TransactFs.Commit().
	InstallHelper(env *util.Env, progress ProgressFunc) (*PostCommitAction, error)

	// UninstallHelper removes the network helper.
	// Returns PostCommitAction that MUST be called after TransactFs.Commit().
	UninstallHelper(env *util.Env, progress ProgressFunc) (*PostCommitAction, error)
}

// PostCommitAction encapsulates actions that must run after TransactFs.Commit().
type PostCommitAction struct {
	Run func(progress ProgressFunc) error
}

// HelperStatus reports current state of the network helper.
type HelperStatus struct {
	Installed   bool
	NeedsUpdate bool
}

// DetailedStatusInfo provides implementation-specific status for display.
type DetailedStatusInfo struct {
	// DaemonLoaded indicates whether the system daemon is running.
	DaemonLoaded bool
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

// safeProgress returns a no-op ProgressFunc if the given one is nil.
// Use this at the start of functions that accept ProgressFunc to avoid
// repeated nil checks throughout the function body.
func safeProgress(progress ProgressFunc) ProgressFunc {
	if progress == nil {
		return func(string, ...any) {}
	}
	return progress
}
