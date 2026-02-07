// Package network provides network isolation for containers.
// See AGD-027 for the decision to use nftables as the primary Linux network.
// See AGD-028 for the lan-access rule syntax specification.
//
// This package re-exports types from shared/ and provides platform-aware
// factory functions. Callers should import this package, not the subpackages.
package network

import (
	"runtime"

	"github.com/bolasblack/alcatraz/internal/config"
	"github.com/bolasblack/alcatraz/internal/network/shared"
	"github.com/bolasblack/alcatraz/internal/util"
)

// Re-export types from shared package for external callers.
type (
	// Type represents the firewall backend type.
	Type = shared.Type
	// Firewall manages network isolation rules for containers.
	Firewall = shared.Firewall
	// NetworkEnv provides dependency injection for filesystem and command execution.
	NetworkEnv = shared.NetworkEnv
	// NetworkHelper manages platform-specific network isolation.
	NetworkHelper = shared.NetworkHelper
	// PostCommitAction encapsulates actions that must run after TransactFs.Commit().
	PostCommitAction = shared.PostCommitAction
	// HelperStatus reports current state of the network helper.
	HelperStatus = shared.HelperStatus
	// DetailedStatusInfo provides implementation-specific status for display.
	DetailedStatusInfo = shared.DetailedStatusInfo
	// RuleFileInfo describes a single rule file.
	RuleFileInfo = shared.RuleFileInfo
	// ProgressFunc reports progress during operations.
	ProgressFunc = shared.ProgressFunc
	// Protocol represents the transport protocol for a firewall rule.
	Protocol = shared.Protocol
	// LANAccessRule represents a parsed lan-access configuration entry.
	LANAccessRule = shared.LANAccessRule
)

// Re-export constants from shared package.
const (
	TypeNone     = shared.TypeNone
	TypeNFTables = shared.TypeNFTables
	TypePF       = shared.TypePF
	ProtoAll     = shared.ProtoAll
	ProtoTCP     = shared.ProtoTCP
	ProtoUDP     = shared.ProtoUDP
)

// Re-export functions from shared package.
var (
	NewNetworkEnv       = shared.NewNetworkEnv
	NewTestNetworkEnv   = shared.NewTestNetworkEnv
	ParseLANAccessRule  = shared.ParseLANAccessRule
	ParseLANAccessRules = shared.ParseLANAccessRules
	HasAllLAN           = shared.HasAllLAN
)

// Detect returns the available firewall type for the current platform.
func Detect(cmd util.CommandRunner) Type {
	switch runtime.GOOS {
	case "darwin":
		return TypePF
	case "linux":
		if commandExists(cmd, "nft") && nftablesWorking(cmd) {
			return TypeNFTables
		}
		return TypeNone
	default:
		return TypeNone
	}
}

// New creates a Firewall implementation based on the detected type.
// Returns nil if no firewall is available.
func New(env *NetworkEnv) (Firewall, Type) {
	t := Detect(env.Cmd)
	return newFirewallForType(t, env), t
}

// NewNetworkHelper creates a NetworkHelper for the given platform and runtime.
// Returns nil if network isolation is not needed.
func NewNetworkHelper(cfg config.Network, runtimeName string) NetworkHelper {
	return newNetworkHelperForPlatform(cfg, runtimeName)
}

// EnsureFirewallSystemConfig ensures platform-specific system configuration
// exists for the firewall to function. For example, on macOS this ensures the
// pf anchor references exist in /etc/pf.conf.
// Safe to call multiple times (idempotent).
func EnsureFirewallSystemConfig(env *NetworkEnv, fwType Type) error {
	return ensureFirewallSystemConfig(env, fwType)
}

// commandExists checks if a command is available in PATH.
func commandExists(cmd util.CommandRunner, name string) bool {
	_, err := cmd.Run("which", name)
	return err == nil
}

// nftablesWorking checks if nftables kernel support is available.
func nftablesWorking(cmd util.CommandRunner) bool {
	_, err := cmd.Run("nft", "list", "tables")
	return err == nil
}
